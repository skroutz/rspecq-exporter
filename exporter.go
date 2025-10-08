package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "rspecq"
)

// nextTestTimingScript is a Lua script that atomically retrieves the timing
// for the next test in the unprocessed queue. It uses EVALSHA for optimal performance.
var nextTestTimingScript = redis.NewScript(`
	local unprocessed_key = KEYS[1]
	local timings_key = KEYS[2]

	-- Get the first item from the unprocessed queue (LINDEX is O(1) for index 0)
	local next_test = redis.call('LINDEX', unprocessed_key, 0)

	-- If there's no next test, return nil
	if not next_test then
		return nil
	end

	-- Look up the timing for this test in the global timings ZSET
	local timing = redis.call('ZSCORE', timings_key, next_test)

	-- Return the timing (will be nil if not found)
	return timing
`)

// Build represents a single RSpecQ build and encapsulates build-specific logic
type Build struct {
	id  string
	rdb *redis.Client
}

// RSpecQExporter collects metrics from RSpecQ Redis instance
type RSpecQExporter struct {
	rdb                     *redis.Client
	mutex                   sync.RWMutex
	disablePerWorkerMetrics bool
	buildIDRegex            *regexp.Regexp
	labelNames              []string

	// Build-level metrics
	buildUnprocessed        *prometheus.GaugeVec
	buildRunning            *prometheus.GaugeVec
	buildProcessed          *prometheus.GaugeVec
	buildLost               *prometheus.GaugeVec
	buildExamples           *prometheus.GaugeVec
	buildExampleFailures    *prometheus.GaugeVec
	buildNonExampleErrors   *prometheus.GaugeVec
	buildRequeues           *prometheus.GaugeVec
	buildFlakyFailures      *prometheus.GaugeVec
	buildStatus             *prometheus.GaugeVec
	buildFailFast           *prometheus.GaugeVec
	buildWithdrawnWorkers   *prometheus.GaugeVec
	buildTotalExecutionTime *prometheus.GaugeVec
	buildNextTestTiming     *prometheus.GaugeVec

	// Worker-level metrics
	workerHeartbeats *prometheus.GaugeVec
	buildWorkers     *prometheus.GaugeVec
	workersWithdrawn *prometheus.GaugeVec

	// Timing metrics
	buildElectedMasterAt *prometheus.GaugeVec
	buildReadyAt         *prometheus.GaugeVec
	buildFinishedAt      *prometheus.GaugeVec
	buildDuration        *prometheus.GaugeVec

	// Global metrics
	globalTimings prometheus.Gauge
	runningBuilds prometheus.Gauge

	// Scrape metrics
	scrapeSuccess  prometheus.Gauge
	scrapeDuration prometheus.Gauge
	lastScrapeTime prometheus.Gauge

	// Redis latency metrics
	redisLatency prometheus.Gauge

	// Cached data for metrics
	activeBuilds map[string]bool

	// Metric collections for bulk operations
	// IMPORTANT: When adding a new metric, you must add it to the appropriate collection(s) below
	// in NewRSpecQExporter to ensure it's properly described, collected, and reset.
	allBuildMetrics     []prometheus.Collector // All build-level GaugeVec metrics
	allPerWorkerMetrics []prometheus.Collector // Per-worker metrics (if enabled)
	allScalarMetrics    []prometheus.Collector // Non-vec Gauge metrics
	allResetableMetrics []interface{ Reset() } // Metrics that need Reset() during scraping
}

// NewRSpecQExporter creates a new RSpecQ exporter
func NewRSpecQExporter(rdb *redis.Client, disablePerWorkerMetrics bool, buildIDRegexPattern string) (*RSpecQExporter, error) {
	exporter := &RSpecQExporter{
		rdb:                     rdb,
		disablePerWorkerMetrics: disablePerWorkerMetrics,
		activeBuilds:            make(map[string]bool),
	}

	// Parse and validate regex if provided
	if buildIDRegexPattern != "" {
		re, err := regexp.Compile(buildIDRegexPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid build-id-regex: %w", err)
		}
		exporter.buildIDRegex = re

		// Extract label names from named capture groups
		labelNames := []string{"build_id"} // Always include build_id
		for _, name := range re.SubexpNames() {
			if name != "" {
				labelNames = append(labelNames, name)
			}
		}
		exporter.labelNames = labelNames
	} else {
		// Default: only build_id label
		exporter.labelNames = []string{"build_id"}
	}

	exporter.buildUnprocessed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_unprocessed",
			Help:      "Number of unprocessed jobs in the queue for a build",
		},
		exporter.labelNames,
	)
	exporter.buildRunning = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_running",
			Help:      "Number of jobs currently running for a build",
		},
		exporter.labelNames,
	)
	exporter.buildProcessed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_processed",
			Help:      "Number of processed jobs for a build",
		},
		exporter.labelNames,
	)
	exporter.buildLost = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_lost",
			Help:      "Number of lost jobs for a build",
		},
		exporter.labelNames,
	)
	exporter.buildExamples = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_examples",
			Help:      "Total number of examples executed for a build",
		},
		exporter.labelNames,
	)
	exporter.buildExampleFailures = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_example_failures",
			Help:      "Number of example failures for a build",
		},
		exporter.labelNames,
	)
	exporter.buildNonExampleErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_non_example_errors",
			Help:      "Number of non-example errors (e.g., syntax errors) for a build",
		},
		exporter.labelNames,
	)
	exporter.buildRequeues = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_requeues",
			Help:      "Number of requeued jobs for a build",
		},
		exporter.labelNames,
	)
	exporter.buildFlakyFailures = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_flaky_failures",
			Help:      "Number of flaky failures (examples that failed inconsistently) for a build",
		},
		exporter.labelNames,
	)
	exporter.buildStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_queue_status",
			Help:      "Build queue status (0=inactive, 1=active for each status: initializing, ready, success, failure)",
		},
		append(exporter.labelNames, "status"),
	)
	exporter.buildFailFast = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_fail_fast",
			Help:      "Fail-fast threshold for a build (0 means disabled)",
		},
		exporter.labelNames,
	)
	exporter.buildWithdrawnWorkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_withdrawn_workers",
			Help:      "Total number of withdrawn workers for a build",
		},
		exporter.labelNames,
	)
	exporter.buildTotalExecutionTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_total_execution_time_seconds",
			Help:      "Total execution time for the build in seconds (sum of all worker execution times)",
		},
		exporter.labelNames,
	)
	exporter.buildWorkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_workers",
			Help:      "Number of active workers for a build",
		},
		exporter.labelNames,
	)
	exporter.buildElectedMasterAt = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_elected_master_at",
			Help:      "Timestamp when master worker was elected for a build",
		},
		exporter.labelNames,
	)
	exporter.buildReadyAt = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_ready_at",
			Help:      "Timestamp when build queue was marked ready",
		},
		exporter.labelNames,
	)
	exporter.buildFinishedAt = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_finished_at",
			Help:      "Timestamp when build finished",
		},
		exporter.labelNames,
	)
	exporter.buildDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_duration_seconds",
			Help:      "Build duration in seconds (from ready to finished)",
		},
		exporter.labelNames,
	)
	exporter.buildNextTestTiming = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_next_test_timing_seconds",
			Help:      "Expected execution time in seconds for the next test in the unprocessed queue (retrieved from global timings)",
		},
		exporter.labelNames,
	)
	exporter.globalTimings = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "global_timings",
			Help:      "Number of entries in the global timings key",
		},
	)
	exporter.runningBuilds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "running_builds",
			Help:      "Number of builds currently running (without finished_at timestamp)",
		},
	)
	exporter.scrapeSuccess = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "scrape_success",
			Help:      "Whether the last scrape was successful (1 = success, 0 = failure)",
		},
	)
	exporter.scrapeDuration = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "scrape_duration_seconds",
			Help:      "Duration of the last scrape in seconds",
		},
	)
	exporter.lastScrapeTime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "last_scrape_timestamp",
			Help:      "Unix timestamp of the last scrape",
		},
	)
	exporter.redisLatency = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "redis_latency_ms",
			Help:      "Redis PING latency in milliseconds",
		},
	)

	// Conditionally initialize per-worker metrics
	if !disablePerWorkerMetrics {
		exporter.workerHeartbeats = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_worker_heartbeat_timestamp",
				Help:      "Last heartbeat timestamp for a worker",
			},
			[]string{"build_id", "worker_id"},
		)
		exporter.workersWithdrawn = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_workers_withdrawn",
				Help:      "Number of times a worker was withdrawn (abnormal termination) for a build",
			},
			[]string{"build_id", "worker_id"},
		)
	}

	// Populate metric collections for bulk operations
	// Build-level metrics (all GaugeVec with build labels)
	exporter.allBuildMetrics = []prometheus.Collector{
		exporter.buildUnprocessed,
		exporter.buildRunning,
		exporter.buildProcessed,
		exporter.buildLost,
		exporter.buildExamples,
		exporter.buildExampleFailures,
		exporter.buildNonExampleErrors,
		exporter.buildRequeues,
		exporter.buildFlakyFailures,
		exporter.buildStatus,
		exporter.buildFailFast,
		exporter.buildWithdrawnWorkers,
		exporter.buildWorkers,
		exporter.buildElectedMasterAt,
		exporter.buildReadyAt,
		exporter.buildFinishedAt,
		exporter.buildDuration,
		exporter.buildTotalExecutionTime,
		exporter.buildNextTestTiming,
	}

	// Per-worker metrics (conditionally included)
	if !disablePerWorkerMetrics {
		exporter.allPerWorkerMetrics = []prometheus.Collector{
			exporter.workerHeartbeats,
			exporter.workersWithdrawn,
		}
	}

	// Scalar metrics (Gauge, not GaugeVec)
	exporter.allScalarMetrics = []prometheus.Collector{
		exporter.globalTimings,
		exporter.runningBuilds,
		exporter.scrapeSuccess,
		exporter.scrapeDuration,
		exporter.lastScrapeTime,
		exporter.redisLatency,
	}

	// All metrics that need to be reset during scraping
	exporter.allResetableMetrics = []interface{ Reset() }{
		exporter.buildUnprocessed,
		exporter.buildRunning,
		exporter.buildProcessed,
		exporter.buildLost,
		exporter.buildExamples,
		exporter.buildExampleFailures,
		exporter.buildNonExampleErrors,
		exporter.buildRequeues,
		exporter.buildFlakyFailures,
		exporter.buildStatus,
		exporter.buildFailFast,
		exporter.buildWithdrawnWorkers,
		exporter.buildWorkers,
		exporter.buildElectedMasterAt,
		exporter.buildReadyAt,
		exporter.buildFinishedAt,
		exporter.buildDuration,
		exporter.buildTotalExecutionTime,
		exporter.buildNextTestTiming,
	}

	// Add per-worker metrics to resetable list if enabled
	if !disablePerWorkerMetrics {
		exporter.allResetableMetrics = append(exporter.allResetableMetrics,
			exporter.workerHeartbeats,
			exporter.workersWithdrawn,
		)
	}

	return exporter, nil
}

// Describe implements prometheus.Collector
func (e *RSpecQExporter) Describe(ch chan<- *prometheus.Desc) {
	// Describe all build-level metrics
	for _, metric := range e.allBuildMetrics {
		metric.Describe(ch)
	}

	// Describe per-worker metrics if enabled
	for _, metric := range e.allPerWorkerMetrics {
		metric.Describe(ch)
	}

	// Describe scalar metrics
	for _, metric := range e.allScalarMetrics {
		metric.Describe(ch)
	}
}

// Collect implements prometheus.Collector
func (e *RSpecQExporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	// Collect all build-level metrics
	for _, metric := range e.allBuildMetrics {
		metric.Collect(ch)
	}

	// Collect per-worker metrics if enabled
	for _, metric := range e.allPerWorkerMetrics {
		metric.Collect(ch)
	}

	// Collect scalar metrics
	for _, metric := range e.allScalarMetrics {
		metric.Collect(ch)
	}
}

// StartScraper runs periodic scraping of Redis metrics
func (e *RSpecQExporter) StartScraper(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Do initial scrape
	e.scrape(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.scrape(ctx)
		}
	}
}

// scrape collects metrics from Redis
func (e *RSpecQExporter) scrape(ctx context.Context) {
	start := time.Now()
	success := 1.0

	defer func() {
		duration := time.Since(start).Seconds()
		e.scrapeDuration.Set(duration)
		e.scrapeSuccess.Set(success)
		e.lastScrapeTime.Set(float64(time.Now().Unix()))
	}()

	// Measure Redis latency with PING command
	pingStart := time.Now()
	_, err := e.rdb.Ping(ctx).Result()
	pingDuration := time.Since(pingStart).Milliseconds()
	e.redisLatency.Set(float64(pingDuration))

	if err != nil {
		log.Printf("Redis PING failed: %v", err)
		success = 0.0
		return
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Reset all metrics for clean state using our centralized list
	for _, metric := range e.allResetableMetrics {
		metric.Reset()
	}

	// Discover active builds by scanning for keys
	builds, err := e.discoverBuilds(ctx)
	if err != nil {
		log.Printf("Error discovering builds: %v", err)
		success = 0.0
		return
	}

	// Count running builds (builds without finished_at)
	runningCount := 0

	// Collect metrics for each build
	for _, buildID := range builds {
		isRunning, err := e.collectBuildMetrics(ctx, buildID)
		if err != nil {
			log.Printf("Error collecting metrics for build %s: %v", buildID, err)
			success = 0.0
		}

		// Count running builds
		if isRunning {
			runningCount++
		}
	}

	// Set the running builds metric
	e.runningBuilds.Set(float64(runningCount))

	// Collect global metrics
	if err := e.collectGlobalMetrics(ctx); err != nil {
		log.Printf("Error collecting global metrics: %v", err)
		success = 0.0
	}
}

// discoverBuilds finds all active builds by scanning Redis keys
// Builds are discovered by checking for <build_id>:queue:status keys
func (e *RSpecQExporter) discoverBuilds(ctx context.Context) ([]string, error) {
	builds := make(map[string]bool)

	// Scan for status keys: <build_id>:queue:status
	// This is the only method for discovering active builds
	statusIter := e.rdb.Scan(ctx, 0, "*:queue:status", 1000).Iterator()
	for statusIter.Next(ctx) {
		key := statusIter.Val()
		// Extract build ID from "<build_id>:queue:status"
		// Remove the ":queue:status" suffix to get the build ID
		if strings.HasSuffix(key, ":queue:status") {
			buildID := strings.TrimSuffix(key, ":queue:status")
			builds[buildID] = true
		}
	}
	if err := statusIter.Err(); err != nil {
		return nil, err
	}

	buildList := make([]string, 0, len(builds))
	for buildID := range builds {
		buildList = append(buildList, buildID)
	}

	return buildList, nil
}

// collectBuildMetrics collects all metrics for a specific build
// Returns whether the build is currently running and any error
func (e *RSpecQExporter) collectBuildMetrics(ctx context.Context, buildID string) (bool, error) {
	build := Build{
		id:  buildID,
		rdb: e.rdb,
	}
	return build.CollectMetrics(ctx, e)
}

// extractLabels extracts label values from a build ID using the configured regex
// Returns a map where keys are label names and values are the extracted values
// Always includes "build_id" as a label
func (e *RSpecQExporter) extractLabels(buildID string) prometheus.Labels {
	labels := prometheus.Labels{"build_id": buildID}

	if e.buildIDRegex != nil {
		matches := e.buildIDRegex.FindStringSubmatch(buildID)
		if matches != nil {
			for i, name := range e.buildIDRegex.SubexpNames() {
				if i > 0 && i < len(matches) && name != "" {
					labels[name] = matches[i]
				}
			}
		}
	}

	return labels
}

// CollectMetrics collects all metrics for this build and sets them on the exporter
// Uses Redis MULTI/EXEC to ensure atomic collection of all build metrics
// Returns whether the build is currently running (no finished_at timestamp)
func (b *Build) CollectMetrics(ctx context.Context, e *RSpecQExporter) (bool, error) {
	buildID := b.id

	// Define all keys
	unprocessedKey := buildID + ":queue:unprocessed"
	runningKey := buildID + ":queue:running"
	processedKey := buildID + ":queue:processed"
	lostKey := buildID + ":queue:lost"
	exampleCountKey := buildID + ":example_count"
	failuresKey := buildID + ":example_failures"
	errorsKey := buildID + ":errors"
	requeuesKey := buildID + ":requeues"
	flakyFailuresKey := buildID + ":flaky_failures"
	statusKey := buildID + ":queue:status"
	finishedKey := buildID + ":queue:finished_at"
	configKey := buildID + ":queue:config"
	heartbeatsKey := buildID + ":worker_heartbeats"
	withdrawnKey := buildID + ":workers_withdrawn"
	electedMasterKey := buildID + ":queue:elected_master_at"
	readyKey := buildID + ":queue:ready_at"
	executionTimeKey := buildID + ":build_execution_time_ms"

	// Execute all commands atomically using pipeline with MULTI/EXEC
	pipe := b.rdb.TxPipeline()

	// Queue metrics
	unprocessedCmd := pipe.LLen(ctx, unprocessedKey)
	runningCmd := pipe.HLen(ctx, runningKey)
	processedCmd := pipe.SCard(ctx, processedKey)
	lostCmd := pipe.ZCard(ctx, lostKey)

	// Example metrics
	exampleCountCmd := pipe.Get(ctx, exampleCountKey)
	failuresCmd := pipe.HLen(ctx, failuresKey)
	errorsCmd := pipe.HLen(ctx, errorsKey)
	requeuesCmd := pipe.HLen(ctx, requeuesKey)
	flakyFailuresCmd := pipe.HLen(ctx, flakyFailuresKey)

	// Status metrics
	statusCmd := pipe.Get(ctx, statusKey)

	// Fail-fast config
	failFastCmd := pipe.HGet(ctx, configKey, "fail_fast")

	// Worker metrics
	heartbeatsCmd := pipe.ZRangeWithScores(ctx, heartbeatsKey, 0, -1)
	withdrawnCmd := pipe.HGetAll(ctx, withdrawnKey)

	// Timing metrics
	electedMasterCmd := pipe.Get(ctx, electedMasterKey)
	readyAtCmd := pipe.Get(ctx, readyKey)
	finishedAtCmd := pipe.Get(ctx, finishedKey)
	executionTimeCmd := pipe.Get(ctx, executionTimeKey)

	// Execute the transaction
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return false, fmt.Errorf("failed to execute redis transaction: %w", err)
	}

	// Execute Lua script to get the timing of the next test in the unprocessed queue
	// Uses EVALSHA with automatic fallback to EVAL for optimal performance
	nextTestTiming, err := nextTestTimingScript.Run(ctx, b.rdb, []string{unprocessedKey, "timings"}).Result()
	if err != nil && err != redis.Nil {
		log.Printf("Warning: failed to get next test timing for build %s: %v", buildID, err)
	}

	// Extract labels for this build
	labels := e.extractLabels(buildID)

	// Process results - Queue metrics
	if unprocessed, err := unprocessedCmd.Result(); err == nil {
		e.buildUnprocessed.With(labels).Set(float64(unprocessed))
	}

	if running, err := runningCmd.Result(); err == nil {
		e.buildRunning.With(labels).Set(float64(running))
	}

	if processed, err := processedCmd.Result(); err == nil {
		e.buildProcessed.With(labels).Set(float64(processed))
	}

	if lost, err := lostCmd.Result(); err == nil {
		e.buildLost.With(labels).Set(float64(lost))
	}

	// Process results - Example metrics
	if exampleCount, err := exampleCountCmd.Int64(); err == nil {
		e.buildExamples.With(labels).Set(float64(exampleCount))
	}

	if failures, err := failuresCmd.Result(); err == nil {
		e.buildExampleFailures.With(labels).Set(float64(failures))
	}

	if errors, err := errorsCmd.Result(); err == nil {
		e.buildNonExampleErrors.With(labels).Set(float64(errors))
	}

	if requeues, err := requeuesCmd.Result(); err == nil {
		e.buildRequeues.With(labels).Set(float64(requeues))
	}

	if flakyFailures, err := flakyFailuresCmd.Result(); err == nil {
		e.buildFlakyFailures.With(labels).Set(float64(flakyFailures))
	}

	// Process results - Status metrics
	status, _ := statusCmd.Result()

	// Set all status gauges to 0 initially
	statusLabels := make(prometheus.Labels)
	for k, v := range labels {
		statusLabels[k] = v
	}

	// Reset all possible statuses to 0
	for _, s := range []string{"initializing", "ready", "success", "failure"} {
		statusLabels["status"] = s
		e.buildStatus.With(statusLabels).Set(0)
	}

	// Set the current status to 1
	switch status {
	case "initializing", "ready", "success", "failure":
		statusLabels["status"] = status
		e.buildStatus.With(statusLabels).Set(1)
	}

	// Process results - Fail-fast config
	if failFast, err := failFastCmd.Int64(); err == nil {
		e.buildFailFast.With(labels).Set(float64(failFast))
	}

	// Process results - Worker metrics
	if heartbeats, err := heartbeatsCmd.Result(); err == nil {
		e.buildWorkers.With(labels).Set(float64(len(heartbeats)))
		if !e.disablePerWorkerMetrics {
			for _, hb := range heartbeats {
				workerID := hb.Member.(string)
				e.workerHeartbeats.WithLabelValues(buildID, workerID).Set(hb.Score)
			}
		}
	}

	// Process withdrawn workers - always calculate total count for build-level metric
	if withdrawn, err := withdrawnCmd.Result(); err == nil {
		totalWithdrawn := float64(len(withdrawn))
		e.buildWithdrawnWorkers.With(labels).Set(totalWithdrawn)

		// Set per-worker withdrawn metrics if enabled
		if !e.disablePerWorkerMetrics {
			for workerID, count := range withdrawn {
				if val, err := parseFloat(count); err == nil {
					e.workersWithdrawn.WithLabelValues(buildID, workerID).Set(val)
				}
			}
		}
	}

	// Process results - Timing metrics
	if electedAt, err := electedMasterCmd.Int64(); err == nil {
		e.buildElectedMasterAt.With(labels).Set(float64(electedAt))
	}

	readyAt := int64(0)
	if ra, err := readyAtCmd.Int64(); err == nil {
		readyAt = ra
		e.buildReadyAt.With(labels).Set(float64(readyAt))
	}

	// Process results - Total execution time
	if executionTimeMs, err := executionTimeCmd.Int64(); err == nil {
		// Convert milliseconds to seconds
		executionTimeSecs := float64(executionTimeMs) / 1000.0
		e.buildTotalExecutionTime.With(labels).Set(executionTimeSecs)
	}

	// Process results - Next test timing from Lua script
	if nextTestTiming != nil {
		if timingStr, ok := nextTestTiming.(string); ok {
			if timing, err := parseFloat(timingStr); err == nil {
				e.buildNextTestTiming.With(labels).Set(timing)
			}
		}
	}

	// Check if build is running (no finished_at timestamp)
	isRunning := false
	if finishedAt, err := finishedAtCmd.Int64(); err == nil {
		e.buildFinishedAt.With(labels).Set(float64(finishedAt))

		// Calculate duration if we have ready_at
		if readyAt > 0 {
			duration := finishedAt - readyAt
			e.buildDuration.With(labels).Set(float64(duration))
		}
		// Build has finished_at, so it's not running
		isRunning = false
	} else if err == redis.Nil {
		// No finished_at key means the build is still running
		isRunning = true
	}

	return isRunning, nil
}

// collectGlobalMetrics collects metrics that are not build-specific
func (e *RSpecQExporter) collectGlobalMetrics(ctx context.Context) error {
	// Global timings
	timingsCount, _ := e.rdb.ZCard(ctx, "timings").Result()
	e.globalTimings.Set(float64(timingsCount))

	return nil
}

// parseFloat is a helper to convert string to float64
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
