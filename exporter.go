package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "rspecq"
)

// RSpecQExporter collects metrics from RSpecQ Redis instance
type RSpecQExporter struct {
	rdb   *redis.Client
	mutex sync.RWMutex

	// Build-level metrics
	buildQueueUnprocessed *prometheus.GaugeVec
	buildQueueRunning     *prometheus.GaugeVec
	buildQueueProcessed   *prometheus.GaugeVec
	buildQueueLost        *prometheus.GaugeVec
	buildExampleCount     *prometheus.GaugeVec
	buildExampleFailures  *prometheus.GaugeVec
	buildNonExampleErrors *prometheus.GaugeVec
	buildRequeues         *prometheus.GaugeVec
	buildStatus           *prometheus.GaugeVec
	buildFailFast         *prometheus.GaugeVec

	// Worker-level metrics
	workerHeartbeats      *prometheus.GaugeVec
	workerCount           *prometheus.GaugeVec
	workersWithdrawn      *prometheus.GaugeVec

	// Timing metrics
	buildElectedMasterAt  *prometheus.GaugeVec
	buildReadyAt          *prometheus.GaugeVec
	buildFinishedAt       *prometheus.GaugeVec
	buildDuration         *prometheus.GaugeVec

	// Global metrics
	globalTimingsCount    prometheus.Gauge
	buildTimesCount       prometheus.Gauge

	// Scrape metrics
	scrapeSuccess         prometheus.Gauge
	scrapeDuration        prometheus.Gauge
	lastScrapeTime        prometheus.Gauge

	// Cached data for metrics
	activeBuilds map[string]bool
}

// NewRSpecQExporter creates a new RSpecQ exporter
func NewRSpecQExporter(rdb *redis.Client) *RSpecQExporter {
	return &RSpecQExporter{
		rdb: rdb,
		buildQueueUnprocessed: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_queue_unprocessed",
				Help:      "Number of unprocessed jobs in the queue for a build",
			},
			[]string{"build_id"},
		),
		buildQueueRunning: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_queue_running",
				Help:      "Number of jobs currently running for a build",
			},
			[]string{"build_id"},
		),
		buildQueueProcessed: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_queue_processed",
				Help:      "Number of processed jobs for a build",
			},
			[]string{"build_id"},
		),
		buildQueueLost: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_queue_lost",
				Help:      "Number of lost jobs for a build",
			},
			[]string{"build_id"},
		),
		buildExampleCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_example_count",
				Help:      "Total number of examples executed for a build",
			},
			[]string{"build_id"},
		),
		buildExampleFailures: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_example_failures",
				Help:      "Number of example failures for a build",
			},
			[]string{"build_id"},
		),
		buildNonExampleErrors: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_non_example_errors",
				Help:      "Number of non-example errors (e.g., syntax errors) for a build",
			},
			[]string{"build_id"},
		),
		buildRequeues: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_requeues",
				Help:      "Number of requeued jobs for a build",
			},
			[]string{"build_id"},
		),
		buildStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_status",
				Help:      "Build status (0=initializing, 1=ready, 2=finished, 3=failed)",
			},
			[]string{"build_id", "status"},
		),
		buildFailFast: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_fail_fast",
				Help:      "Fail-fast threshold for a build (0 means disabled)",
			},
			[]string{"build_id"},
		),
		workerHeartbeats: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "worker_heartbeat_timestamp",
				Help:      "Last heartbeat timestamp for a worker",
			},
			[]string{"build_id", "worker_id"},
		),
		workerCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "worker_count",
				Help:      "Number of active workers for a build",
			},
			[]string{"build_id"},
		),
		workersWithdrawn: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "workers_withdrawn",
				Help:      "Number of times a worker was withdrawn (abnormal termination) for a build",
			},
			[]string{"build_id", "worker_id"},
		),
		buildElectedMasterAt: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_elected_master_at",
				Help:      "Timestamp when master worker was elected for a build",
			},
			[]string{"build_id"},
		),
		buildReadyAt: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_ready_at",
				Help:      "Timestamp when build queue was marked ready",
			},
			[]string{"build_id"},
		),
		buildFinishedAt: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_finished_at",
				Help:      "Timestamp when build finished",
			},
			[]string{"build_id"},
		),
		buildDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_duration_seconds",
				Help:      "Build duration in seconds (from ready to finished)",
			},
			[]string{"build_id"},
		),
		globalTimingsCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "global_timings_count",
				Help:      "Number of entries in the global timings key",
			},
		),
		buildTimesCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_times_count",
				Help:      "Number of build time entries stored",
			},
		),
		scrapeSuccess: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "scrape_success",
				Help:      "Whether the last scrape was successful (1 = success, 0 = failure)",
			},
		),
		scrapeDuration: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "scrape_duration_seconds",
				Help:      "Duration of the last scrape in seconds",
			},
		),
		lastScrapeTime: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "last_scrape_timestamp",
				Help:      "Unix timestamp of the last scrape",
			},
		),
		activeBuilds: make(map[string]bool),
	}
}

// Describe implements prometheus.Collector
func (e *RSpecQExporter) Describe(ch chan<- *prometheus.Desc) {
	e.buildQueueUnprocessed.Describe(ch)
	e.buildQueueRunning.Describe(ch)
	e.buildQueueProcessed.Describe(ch)
	e.buildQueueLost.Describe(ch)
	e.buildExampleCount.Describe(ch)
	e.buildExampleFailures.Describe(ch)
	e.buildNonExampleErrors.Describe(ch)
	e.buildRequeues.Describe(ch)
	e.buildStatus.Describe(ch)
	e.buildFailFast.Describe(ch)
	e.workerHeartbeats.Describe(ch)
	e.workerCount.Describe(ch)
	e.workersWithdrawn.Describe(ch)
	e.buildElectedMasterAt.Describe(ch)
	e.buildReadyAt.Describe(ch)
	e.buildFinishedAt.Describe(ch)
	e.buildDuration.Describe(ch)
	e.globalTimingsCount.Describe(ch)
	e.buildTimesCount.Describe(ch)
	e.scrapeSuccess.Describe(ch)
	e.scrapeDuration.Describe(ch)
	e.lastScrapeTime.Describe(ch)
}

// Collect implements prometheus.Collector
func (e *RSpecQExporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	e.buildQueueUnprocessed.Collect(ch)
	e.buildQueueRunning.Collect(ch)
	e.buildQueueProcessed.Collect(ch)
	e.buildQueueLost.Collect(ch)
	e.buildExampleCount.Collect(ch)
	e.buildExampleFailures.Collect(ch)
	e.buildNonExampleErrors.Collect(ch)
	e.buildRequeues.Collect(ch)
	e.buildStatus.Collect(ch)
	e.buildFailFast.Collect(ch)
	e.workerHeartbeats.Collect(ch)
	e.workerCount.Collect(ch)
	e.workersWithdrawn.Collect(ch)
	e.buildElectedMasterAt.Collect(ch)
	e.buildReadyAt.Collect(ch)
	e.buildFinishedAt.Collect(ch)
	e.buildDuration.Collect(ch)
	e.globalTimingsCount.Collect(ch)
	e.buildTimesCount.Collect(ch)
	e.scrapeSuccess.Collect(ch)
	e.scrapeDuration.Collect(ch)
	e.lastScrapeTime.Collect(ch)
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

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Reset metrics for clean state
	e.buildQueueUnprocessed.Reset()
	e.buildQueueRunning.Reset()
	e.buildQueueProcessed.Reset()
	e.buildQueueLost.Reset()
	e.buildExampleCount.Reset()
	e.buildExampleFailures.Reset()
	e.buildNonExampleErrors.Reset()
	e.buildRequeues.Reset()
	e.buildStatus.Reset()
	e.buildFailFast.Reset()
	e.workerHeartbeats.Reset()
	e.workerCount.Reset()
	e.workersWithdrawn.Reset()
	e.buildElectedMasterAt.Reset()
	e.buildReadyAt.Reset()
	e.buildFinishedAt.Reset()
	e.buildDuration.Reset()

	// Discover active builds by scanning for keys
	builds, err := e.discoverBuilds(ctx)
	if err != nil {
		log.Printf("Error discovering builds: %v", err)
		success = 0.0
		return
	}

	// Collect metrics for each build
	for _, buildID := range builds {
		if err := e.collectBuildMetrics(ctx, buildID); err != nil {
			log.Printf("Error collecting metrics for build %s: %v", buildID, err)
			success = 0.0
		}
	}

	// Collect global metrics
	if err := e.collectGlobalMetrics(ctx); err != nil {
		log.Printf("Error collecting global metrics: %v", err)
		success = 0.0
	}
}

// discoverBuilds finds all active builds by scanning Redis keys
func (e *RSpecQExporter) discoverBuilds(ctx context.Context) ([]string, error) {
	builds := make(map[string]bool)
	
	// Scan for build-specific keys
	// Pattern: <build_id>:*
	iter := e.rdb.Scan(ctx, 0, "*:queue:*", 1000).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		parts := strings.Split(key, ":")
		if len(parts) >= 2 {
			buildID := parts[0]
			builds[buildID] = true
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	buildList := make([]string, 0, len(builds))
	for buildID := range builds {
		buildList = append(buildList, buildID)
	}
	
	return buildList, nil
}

// collectBuildMetrics collects all metrics for a specific build
func (e *RSpecQExporter) collectBuildMetrics(ctx context.Context, buildID string) error {
	// Queue metrics
	unprocessedKey := buildID + ":queue:unprocessed"
	runningKey := buildID + ":queue:running"
	processedKey := buildID + ":queue:processed"
	lostKey := buildID + ":queue:lost"
	
	unprocessed, _ := e.rdb.LLen(ctx, unprocessedKey).Result()
	e.buildQueueUnprocessed.WithLabelValues(buildID).Set(float64(unprocessed))
	
	running, _ := e.rdb.HLen(ctx, runningKey).Result()
	e.buildQueueRunning.WithLabelValues(buildID).Set(float64(running))
	
	processed, _ := e.rdb.SCard(ctx, processedKey).Result()
	e.buildQueueProcessed.WithLabelValues(buildID).Set(float64(processed))
	
	lost, _ := e.rdb.ZCard(ctx, lostKey).Result()
	e.buildQueueLost.WithLabelValues(buildID).Set(float64(lost))

	// Example metrics
	exampleCountKey := buildID + ":example_count"
	failuresKey := buildID + ":example_failures"
	errorsKey := buildID + ":errors"
	requeuesKey := buildID + ":requeues"
	
	exampleCount, _ := e.rdb.Get(ctx, exampleCountKey).Int64()
	e.buildExampleCount.WithLabelValues(buildID).Set(float64(exampleCount))
	
	failures, _ := e.rdb.HLen(ctx, failuresKey).Result()
	e.buildExampleFailures.WithLabelValues(buildID).Set(float64(failures))
	
	errors, _ := e.rdb.HLen(ctx, errorsKey).Result()
	e.buildNonExampleErrors.WithLabelValues(buildID).Set(float64(errors))
	
	requeues, _ := e.rdb.HLen(ctx, requeuesKey).Result()
	e.buildRequeues.WithLabelValues(buildID).Set(float64(requeues))

	// Status metrics
	statusKey := buildID + ":queue:status"
	status, _ := e.rdb.Get(ctx, statusKey).Result()
	
	// Set status gauges
	e.buildStatus.WithLabelValues(buildID, "initializing").Set(0)
	e.buildStatus.WithLabelValues(buildID, "ready").Set(0)
	e.buildStatus.WithLabelValues(buildID, "finished").Set(0)
	e.buildStatus.WithLabelValues(buildID, "failed").Set(0)
	
	switch status {
	case "initializing":
		e.buildStatus.WithLabelValues(buildID, "initializing").Set(1)
	case "ready":
		e.buildStatus.WithLabelValues(buildID, "ready").Set(1)
	}
	
	// Check if finished or failed
	finishedKey := buildID + ":queue:finished_at"
	if exists, _ := e.rdb.Exists(ctx, finishedKey).Result(); exists > 0 {
		if failures > 0 || errors > 0 {
			e.buildStatus.WithLabelValues(buildID, "failed").Set(1)
		} else {
			e.buildStatus.WithLabelValues(buildID, "finished").Set(1)
		}
	}

	// Fail-fast config
	configKey := buildID + ":queue:config"
	failFast, _ := e.rdb.HGet(ctx, configKey, "fail_fast").Int64()
	e.buildFailFast.WithLabelValues(buildID).Set(float64(failFast))

	// Worker metrics
	heartbeatsKey := buildID + ":worker_heartbeats"
	heartbeats, _ := e.rdb.ZRangeWithScores(ctx, heartbeatsKey, 0, -1).Result()
	e.workerCount.WithLabelValues(buildID).Set(float64(len(heartbeats)))
	
	for _, hb := range heartbeats {
		workerID := hb.Member.(string)
		e.workerHeartbeats.WithLabelValues(buildID, workerID).Set(hb.Score)
	}

	// Workers withdrawn
	withdrawnKey := buildID + ":workers_withdrawn"
	withdrawn, _ := e.rdb.HGetAll(ctx, withdrawnKey).Result()
	for workerID, count := range withdrawn {
		if val, err := parseFloat(count); err == nil {
			e.workersWithdrawn.WithLabelValues(buildID, workerID).Set(val)
		}
	}

	// Timing metrics
	electedMasterKey := buildID + ":queue:elected_master_at"
	if electedAt, err := e.rdb.Get(ctx, electedMasterKey).Int64(); err == nil {
		e.buildElectedMasterAt.WithLabelValues(buildID).Set(float64(electedAt))
	}
	
	readyKey := buildID + ":queue:ready_at"
	if readyAt, err := e.rdb.Get(ctx, readyKey).Int64(); err == nil {
		e.buildReadyAt.WithLabelValues(buildID).Set(float64(readyAt))
	}
	
	if finishedAt, err := e.rdb.Get(ctx, finishedKey).Int64(); err == nil {
		e.buildFinishedAt.WithLabelValues(buildID).Set(float64(finishedAt))
		
		// Calculate duration if we have ready_at
		if readyAt, err := e.rdb.Get(ctx, readyKey).Int64(); err == nil {
			duration := finishedAt - readyAt
			e.buildDuration.WithLabelValues(buildID).Set(float64(duration))
		}
	}

	return nil
}

// collectGlobalMetrics collects metrics that are not build-specific
func (e *RSpecQExporter) collectGlobalMetrics(ctx context.Context) error {
	// Global timings
	timingsCount, _ := e.rdb.ZCard(ctx, "timings").Result()
	e.globalTimingsCount.Set(float64(timingsCount))
	
	// Build times
	buildTimesCount, _ := e.rdb.LLen(ctx, "build_times").Result()
	e.buildTimesCount.Set(float64(buildTimesCount))
	
	return nil
}

// parseFloat is a helper to convert string to float64
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
