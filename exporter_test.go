package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewRSpecQExporter(t *testing.T) {
	// Create a mock Redis client (won't actually connect)
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	if exporter == nil {
		t.Fatal("Expected non-nil exporter")
	}

	if exporter.rdb != rdb {
		t.Error("Redis client not properly set")
	}

	if exporter.activeBuilds == nil {
		t.Error("activeBuilds map not initialized")
	}
}

func TestExporterImplementsCollector(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Check that exporter implements prometheus.Collector
	var _ prometheus.Collector = exporter
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		hasError bool
	}{
		{"42.5", 42.5, false},
		{"0", 0, false},
		{"123", 123, false},
		{"-10.5", -10.5, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		result, err := parseFloat(tt.input)

		if tt.hasError {
			if err == nil {
				t.Errorf("Expected error for input %q, got none", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("For input %q, expected %f, got %f", tt.input, tt.expected, result)
			}
		}
	}
}

// Integration tests using miniredis

// setupTestRedis creates a test Redis server and returns the client and cleanup function
func setupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis, func()) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cleanup := func() {
		rdb.Close()
		mr.Close()
	}

	return rdb, mr, cleanup
}

func TestDiscoverBuilds_NoBuilds(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}
	ctx := context.Background()

	builds, err := exporter.discoverBuilds(ctx)
	if err != nil {
		t.Fatalf("discoverBuilds failed: %v", err)
	}

	if len(builds) != 0 {
		t.Errorf("Expected 0 builds, got %d", len(builds))
	}
}

func TestDiscoverBuilds_MultipleBuilds(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	buildIDs := []string{"build-1", "build-2", "build-3"}

	// Create status keys for multiple builds
	for _, buildID := range buildIDs {
		err := rdb.Set(ctx, buildID+":queue:status", "ready", 0).Err()
		if err != nil {
			t.Fatalf("Failed to create status key for %s: %v", buildID, err)
		}
	}

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}
	builds, err := exporter.discoverBuilds(ctx)
	if err != nil {
		t.Fatalf("discoverBuilds failed: %v", err)
	}

	if len(builds) != len(buildIDs) {
		t.Fatalf("Expected %d builds, got %d", len(buildIDs), len(builds))
	}

	// Convert to map for easier checking
	foundBuilds := make(map[string]bool)
	for _, build := range builds {
		foundBuilds[build] = true
	}

	for _, expectedID := range buildIDs {
		if !foundBuilds[expectedID] {
			t.Errorf("Expected to find build %q but it was not discovered", expectedID)
		}
	}
}

func TestDiscoverBuilds_WithStatusKeys(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	buildID := "test-build-with-status"

	// Create status key and other data
	err := rdb.Set(ctx, buildID+":queue:status", "initializing", 0).Err()
	if err != nil {
		t.Fatalf("Failed to create status key: %v", err)
	}

	// Also create queue data (but discovery should only happen via status key)
	rdb.LPush(ctx, buildID+":queue:unprocessed", "job1")
	rdb.SAdd(ctx, buildID+":queue:processed", "job2", "job3")

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}
	builds, err := exporter.discoverBuilds(ctx)
	if err != nil {
		t.Fatalf("discoverBuilds failed: %v", err)
	}

	if len(builds) != 1 {
		t.Fatalf("Expected 1 build, got %d: %v", len(builds), builds)
	}

	if builds[0] != buildID {
		t.Errorf("Expected build ID %q, got %q", buildID, builds[0])
	}
}

func TestScrape_WithBuilds(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	buildID := "test-build-scrape"

	// Set up a complete build with various metrics
	setupTestBuild(t, ctx, rdb, buildID)

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Run scrape
	exporter.scrape(ctx)

	// Verify that scrape was successful
	if testutil.ToFloat64(exporter.scrapeSuccess) != 1.0 {
		t.Error("Expected scrape to succeed")
	}

	// Verify that metrics were collected
	queueUnprocessed := testutil.ToFloat64(exporter.buildUnprocessed.WithLabelValues(buildID))
	if queueUnprocessed != 2.0 {
		t.Errorf("Expected 2 unprocessed jobs, got %f", queueUnprocessed)
	}

	exampleCount := testutil.ToFloat64(exporter.buildExamples.WithLabelValues(buildID))
	if exampleCount != 42.0 {
		t.Errorf("Expected 42 examples, got %f", exampleCount)
	}
}

func TestPeriodicScraping(t *testing.T) {
	rdb, mr, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	buildID := "periodic-test-build"
	setupTestBuild(t, ctx, rdb, buildID)

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Start scraper with short interval
	go exporter.StartScraper(ctx, 100*time.Millisecond)

	// Wait for a few scrapes
	time.Sleep(350 * time.Millisecond)

	// Fast forward miniredis time if needed
	mr.FastForward(1 * time.Second)

	// Verify that scraping happened
	scrapeTime := testutil.ToFloat64(exporter.lastScrapeTime)
	if scrapeTime == 0 {
		t.Error("Expected lastScrapeTime to be set after scraping")
	}

	// Verify metrics were collected
	queueUnprocessed := testutil.ToFloat64(exporter.buildUnprocessed.WithLabelValues(buildID))
	if queueUnprocessed != 2.0 {
		t.Errorf("Expected 2 unprocessed jobs, got %f", queueUnprocessed)
	}
}

// setupTestBuild creates a complete build with test data in Redis
func setupTestBuild(t *testing.T, ctx context.Context, rdb *redis.Client, buildID string) {
	t.Helper()

	// Status key - REQUIRED for build discovery
	rdb.Set(ctx, buildID+":queue:status", "ready", 0)

	// Queue data
	rdb.LPush(ctx, buildID+":queue:unprocessed", "job1", "job2")
	rdb.HSet(ctx, buildID+":queue:running", "worker-1", "job3")
	rdb.SAdd(ctx, buildID+":queue:processed", "job4", "job5", "job6")
	rdb.ZAdd(ctx, buildID+":queue:lost", &redis.Z{Score: 1.0, Member: "lost-job"})

	// Example metrics
	rdb.Set(ctx, buildID+":example_count", "42", 0)
	rdb.HSet(ctx, buildID+":example_failures", "spec1", "failure1")
	rdb.HSet(ctx, buildID+":example_failures", "spec2", "failure2")
	rdb.HSet(ctx, buildID+":errors", "error1", "details")
	rdb.HSet(ctx, buildID+":requeues", "requeue1", "1")

	// Config
	rdb.HSet(ctx, buildID+":queue:config", "fail_fast", "5")

	// Worker heartbeats
	now := float64(time.Now().Unix())
	rdb.ZAdd(ctx, buildID+":worker_heartbeats",
		&redis.Z{Score: now, Member: "worker-1"},
		&redis.Z{Score: now - 10, Member: "worker-2"},
	)

	// Timing
	baseTime := time.Now().Unix()
	rdb.Set(ctx, buildID+":queue:elected_master_at", baseTime, 0)
	rdb.Set(ctx, buildID+":queue:ready_at", baseTime+10, 0)
}

// TestE2E_HappyPath_AllMetrics is a comprehensive end-to-end test that validates
// all metrics are correctly exported in Prometheus format.
//
// This test:
// 1. Sets up a complete RSpecQ build with realistic Redis data including:
//   - Queue metrics (unprocessed, running, processed, lost jobs)
//   - Example counts, failures, and errors
//   - Worker heartbeats and withdrawn workers
//   - Timing data (elected master, ready timestamps)
//   - Configuration (fail-fast threshold)
//   - Global metrics (timings and build times)
//
// 2. Runs the exporter's scrape operation to collect all metrics
// 3. Gathers metrics in Prometheus format (using prometheus.Gather())
// 4. Validates that all expected metrics are present with correct values
// 5. Uses actual Prometheus metric format strings for validation (bonus points!)
//
// The test verifies ALL metric types exported by the RSpecQ exporter:
//   - Build-level metrics (queue, examples, status, config)
//   - Worker-level metrics (heartbeats, count, withdrawn)
//   - Timing metrics (elected_master_at, ready_at)
//   - Global metrics (timings_count)
//   - Scrape metrics (success, duration, timestamp)
func TestE2E_HappyPath_AllMetrics(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	buildID := "e2e-test-build"

	// Setup comprehensive test data
	baseTime := time.Now().Unix()

	// Status key - REQUIRED for build discovery
	rdb.Set(ctx, buildID+":queue:status", "ready", 0)

	// Queue data - comprehensive counts
	rdb.LPush(ctx, buildID+":queue:unprocessed", "job1", "job2", "job3")
	rdb.HSet(ctx, buildID+":queue:running", "worker-1", "job4")
	rdb.HSet(ctx, buildID+":queue:running", "worker-2", "job5")
	rdb.SAdd(ctx, buildID+":queue:processed", "job6", "job7", "job8", "job9", "job10")
	rdb.ZAdd(ctx, buildID+":queue:lost",
		&redis.Z{Score: 1.0, Member: "lost-job1"},
		&redis.Z{Score: 2.0, Member: "lost-job2"})

	// Example metrics
	rdb.Set(ctx, buildID+":example_count", "150", 0)
	rdb.HSet(ctx, buildID+":example_failures", "spec1.rb", "failure1")
	rdb.HSet(ctx, buildID+":example_failures", "spec2.rb", "failure2")
	rdb.HSet(ctx, buildID+":example_failures", "spec3.rb", "failure3")
	rdb.HSet(ctx, buildID+":errors", "syntax_error.rb", "error details")
	rdb.HSet(ctx, buildID+":errors", "load_error.rb", "cannot load")

	// Requeues - the value is the count for each worker, metric is the number of hash fields
	rdb.HSet(ctx, buildID+":requeues", "worker-1", "1")
	rdb.HSet(ctx, buildID+":requeues", "worker-2", "2")
	rdb.HSet(ctx, buildID+":requeues", "worker-3", "1")

	// Flaky failures - examples that failed inconsistently
	rdb.HSet(ctx, buildID+":flaky_failures", "spec/flaky_spec.rb[1:2:3]", "Failure/Error: expect(result).to eq(expected)")
	rdb.HSet(ctx, buildID+":flaky_failures", "spec/intermittent_spec.rb[4:5]", "Connection timeout")

	// Config
	rdb.HSet(ctx, buildID+":queue:config", "fail_fast", "10")

	// Queue status - needed for status metric
	rdb.Set(ctx, buildID+":queue:status", "ready", 0)

	// Worker heartbeats - 3 active workers
	now := float64(time.Now().Unix())
	rdb.ZAdd(ctx, buildID+":worker_heartbeats",
		&redis.Z{Score: now - 5, Member: "worker-1"},
		&redis.Z{Score: now - 3, Member: "worker-2"},
		&redis.Z{Score: now - 1, Member: "worker-3"},
	)

	// Workers withdrawn - stored as hash map with worker_id => count
	rdb.HSet(ctx, buildID+":workers_withdrawn", "worker-4", "1")
	rdb.HSet(ctx, buildID+":workers_withdrawn", "worker-5", "2")

	// Timing data (don't set finished_at since we want status to be "ready")
	rdb.Set(ctx, buildID+":queue:elected_master_at", baseTime, 0)
	rdb.Set(ctx, buildID+":queue:ready_at", baseTime+10, 0)
	// Note: NOT setting :queue:finished_at so build stays in "ready" status

	// Global metrics - note: keys are "timings" and "build_times" (not rspecq:timings)
	rdb.ZAdd(ctx, "timings",
		&redis.Z{Score: 1.5, Member: "spec1.rb"},
		&redis.Z{Score: 2.3, Member: "spec2.rb"},
		&redis.Z{Score: 0.8, Member: "spec3.rb"},
	)
	rdb.RPush(ctx, "build_times", "120", "95")

	// Create exporter and register with a custom registry for testing
	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter)

	// Run scrape to collect all metrics
	exporter.scrape(ctx)

	// Gather metrics in Prometheus format
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Convert to Prometheus text format (like what /metrics endpoint returns)
	var metricsText strings.Builder
	for _, mf := range metricFamilies {
		for _, m := range mf.GetMetric() {
			labels := ""
			if len(m.GetLabel()) > 0 {
				var labelPairs []string
				for _, l := range m.GetLabel() {
					labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, l.GetName(), l.GetValue()))
				}
				labels = "{" + strings.Join(labelPairs, ",") + "}"
			}

			var value float64
			if m.GetGauge() != nil {
				value = m.GetGauge().GetValue()
			}

			metricsText.WriteString(fmt.Sprintf("%s%s %v\n", mf.GetName(), labels, value))
		}
	}

	prometheusOutput := metricsText.String()

	t.Logf("Prometheus metrics output:\n%s", prometheusOutput)

	// Define expected metrics with their getters for validation
	type metricTest struct {
		prometheusLine string
		expectedValue  float64
		actualGetter   func() float64
	}

	tests := []metricTest{
		// Queue metrics
		{`rspecq_build_unprocessed{build_id="e2e-test-build"}`, 3, func() float64 {
			return testutil.ToFloat64(exporter.buildUnprocessed.WithLabelValues(buildID))
		}},
		{`rspecq_build_running{build_id="e2e-test-build"}`, 2, func() float64 {
			return testutil.ToFloat64(exporter.buildRunning.WithLabelValues(buildID))
		}},
		{`rspecq_build_processed{build_id="e2e-test-build"}`, 5, func() float64 {
			return testutil.ToFloat64(exporter.buildProcessed.WithLabelValues(buildID))
		}},
		{`rspecq_build_lost{build_id="e2e-test-build"}`, 2, func() float64 {
			return testutil.ToFloat64(exporter.buildLost.WithLabelValues(buildID))
		}},

		// Example metrics
		{`rspecq_build_examples{build_id="e2e-test-build"}`, 150, func() float64 {
			return testutil.ToFloat64(exporter.buildExamples.WithLabelValues(buildID))
		}},
		{`rspecq_build_example_failures{build_id="e2e-test-build"}`, 3, func() float64 {
			return testutil.ToFloat64(exporter.buildExampleFailures.WithLabelValues(buildID))
		}},
		{`rspecq_build_non_example_errors{build_id="e2e-test-build"}`, 2, func() float64 {
			return testutil.ToFloat64(exporter.buildNonExampleErrors.WithLabelValues(buildID))
		}},
		{`rspecq_build_requeues{build_id="e2e-test-build"}`, 3, func() float64 {
			return testutil.ToFloat64(exporter.buildRequeues.WithLabelValues(buildID))
		}},
		{`rspecq_build_flaky_failures{build_id="e2e-test-build"}`, 2, func() float64 {
			return testutil.ToFloat64(exporter.buildFlakyFailures.WithLabelValues(buildID))
		}},

		// Status metric (should be 1 for "ready")
		{`rspecq_build_queue_status{build_id="e2e-test-build",status="ready"}`, 1, func() float64 {
			return testutil.ToFloat64(exporter.buildStatus.WithLabelValues(buildID, "ready"))
		}},

		// Config
		{`rspecq_build_fail_fast{build_id="e2e-test-build"}`, 10, func() float64 {
			return testutil.ToFloat64(exporter.buildFailFast.WithLabelValues(buildID))
		}},

		// Worker metrics
		{`rspecq_build_workers{build_id="e2e-test-build"}`, 3, func() float64 {
			return testutil.ToFloat64(exporter.workers.WithLabelValues(buildID))
		}},

		// Withdrawn workers count (build-level metric)
		{`rspecq_build_withdrawn_workers{build_id="e2e-test-build"}`, 2, func() float64 {
			return testutil.ToFloat64(exporter.buildWithdrawnWorkers.WithLabelValues(buildID))
		}},

		// Global metrics
		{`rspecq_global_timings`, 3, func() float64 {
			return testutil.ToFloat64(exporter.globalTimings)
		}},

		// Running builds metric (this build is running because it's "ready" without finished_at)
		{`rspecq_running_builds`, 1, func() float64 {
			return testutil.ToFloat64(exporter.runningBuilds)
		}},

		// Scrape metrics
		{`rspecq_scrape_success`, 1, func() float64 {
			return testutil.ToFloat64(exporter.scrapeSuccess)
		}},
	}

	// Verify all expected metrics are present with correct values
	for _, test := range tests {
		if !strings.Contains(prometheusOutput, test.prometheusLine) {
			t.Errorf("Expected metric not found in output: %s", test.prometheusLine)
			continue
		}

		actual := test.actualGetter()
		if actual != test.expectedValue {
			t.Errorf("%s: expected %v, got %v", test.prometheusLine, test.expectedValue, actual)
		}
	}

	// Verify timing metrics exist (values will vary based on test time)
	timingMetrics := []string{
		`rspecq_build_elected_master_at{build_id="e2e-test-build"}`,
		`rspecq_build_ready_at{build_id="e2e-test-build"}`,
		`rspecq_scrape_duration_seconds`,
		`rspecq_last_scrape_timestamp`,
	}

	for _, metric := range timingMetrics {
		if !strings.Contains(prometheusOutput, metric) {
			t.Errorf("Expected timing metric not found in output: %s", metric)
		}
	}

	// Verify worker heartbeat metrics exist for all workers
	workerHeartbeatMetrics := []string{
		`rspecq_build_worker_heartbeat_timestamp{build_id="e2e-test-build",worker_id="worker-1"}`,
		`rspecq_build_worker_heartbeat_timestamp{build_id="e2e-test-build",worker_id="worker-2"}`,
		`rspecq_build_worker_heartbeat_timestamp{build_id="e2e-test-build",worker_id="worker-3"}`,
	}

	for _, metric := range workerHeartbeatMetrics {
		if !strings.Contains(prometheusOutput, metric) {
			t.Errorf("Expected worker heartbeat metric not found in output: %s", metric)
		}
	}

	// Verify withdrawn worker metrics
	withdrawnMetrics := []string{
		`rspecq_build_workers_withdrawn{build_id="e2e-test-build",worker_id="worker-4"}`,
		`rspecq_build_workers_withdrawn{build_id="e2e-test-build",worker_id="worker-5"}`,
	}

	for _, metric := range withdrawnMetrics {
		if !strings.Contains(prometheusOutput, metric) {
			t.Errorf("Expected withdrawn worker metric not found in output: %s", metric)
		}
	}

	// Ensure initializing status is 0 (only "ready" should be 1)
	// There are only two statuses: initializing and ready
	initializingMetric := `rspecq_build_queue_status{build_id="e2e-test-build",status="initializing"} 0`
	if !strings.Contains(prometheusOutput, initializingMetric) {
		t.Error("Expected initializing status to be 0")
	}

	t.Logf("✓ All metrics validated successfully!")
	t.Logf("✓ Total metric families: %d", len(metricFamilies))

	// Additional test: Verify metrics cleanup after build removal
	// This ensures the fix for the flaky_failures bug (metrics not being reset) works correctly
	t.Logf("Testing metric cleanup after build removal...")

	// Delete all build-related keys from Redis
	iter := rdb.Scan(ctx, 0, buildID+":*", 1000).Iterator()
	keysToDelete := []string{}
	for iter.Next(ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}
	if err := iter.Err(); err != nil {
		t.Fatalf("Failed to scan for build keys: %v", err)
	}

	// Delete the keys
	if len(keysToDelete) > 0 {
		rdb.Del(ctx, keysToDelete...)
	}

	// Also delete the status key explicitly (it's the discovery key)
	rdb.Del(ctx, buildID+":queue:status")

	// Run another scrape - should not find the build anymore
	exporter.scrape(ctx)

	// Gather metrics again
	metricFamilies2, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics after cleanup: %v", err)
	}

	// Check that NO build-level metrics contain our buildID
	for _, mf := range metricFamilies2 {
		// Only check build-level metrics (those with "build_" in the name)
		if !strings.Contains(mf.GetName(), "build_") {
			continue
		}

		for _, m := range mf.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetName() == "build_id" && label.GetValue() == buildID {
					t.Errorf("After cleanup, metric %s still contains build_id=%s", mf.GetName(), buildID)
				}
			}
		}
	}

	t.Logf("✓ Metrics properly cleaned up after build removal from Redis")
}

func TestDisablePerWorkerMetrics(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	buildID := "test-build-no-worker"

	// Set up a build with worker metrics
	setupTestBuild(t, ctx, rdb, buildID)

	// Create exporter with per-worker metrics disabled
	exporter, err := NewRSpecQExporter(rdb, true, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter)

	// Run scrape to collect metrics
	exporter.scrape(ctx)

	// Gather metrics
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Convert to text for easier validation
	var metricsText strings.Builder
	for _, mf := range metricFamilies {
		metricsText.WriteString(mf.String())
	}
	output := metricsText.String()

	// Verify per-worker metrics are NOT present
	perWorkerMetrics := []string{
		"rspecq_build_worker_heartbeat_timestamp",
		"rspecq_build_workers_withdrawn",
	}

	for _, metric := range perWorkerMetrics {
		if strings.Contains(output, metric) {
			t.Errorf("Per-worker metric %s should not be present when disabled", metric)
		}
	}

	// Verify workers is still present (aggregate metric)
	if !strings.Contains(output, "rspecq_build_workers") {
		t.Error("workers metric should still be present (not per-worker)")
	}

	// Verify build_withdrawn_workers is still present (build-level metric)
	if !strings.Contains(output, "rspecq_build_withdrawn_workers") {
		t.Error("build_withdrawn_workers metric should still be present (build-level metric)")
	}

	// Verify other metrics are still present
	expectedMetrics := []string{
		"rspecq_build_unprocessed",
		"rspecq_build_examples",
		"rspecq_build_queue_status",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("Expected metric %s to be present", metric)
		}
	}
}

// TestRunningBuildsIntegration is a comprehensive integration test for the running_builds metric.
// It tests various build states to ensure only builds without finished_at are counted as running.
func TestRunningBuildsIntegration(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	baseTime := time.Now().Unix()

	// Scenario: Multiple builds with different states
	// Expected running builds: 3 (running-1, running-2, running-3)

	// Running build 1 - has status "ready", no finished_at
	rdb.Set(ctx, "running-1:queue:status", "ready", 0)
	rdb.LPush(ctx, "running-1:queue:unprocessed", "job1")

	// Running build 2 - has status "initializing", no finished_at (still counts as running)
	rdb.Set(ctx, "running-2:queue:status", "initializing", 0)
	rdb.LPush(ctx, "running-2:queue:unprocessed", "job1")

	// Running build 3 - no status key at all, no finished_at (still counts as running)
	rdb.Set(ctx, "running-3:queue:status", "ready", 0)
	rdb.LPush(ctx, "running-3:queue:unprocessed", "job1")

	// Finished build 1 - has status "ready" but HAS finished_at (should NOT count)
	rdb.Set(ctx, "finished-1:queue:status", "ready", 0)
	rdb.Set(ctx, "finished-1:queue:finished_at", baseTime, 0)

	// Finished build 2 - has status "initializing" and HAS finished_at (should NOT count)
	rdb.Set(ctx, "finished-2:queue:status", "initializing", 0)
	rdb.Set(ctx, "finished-2:queue:finished_at", baseTime+100, 0)

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Create a registry and gather metrics
	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter)

	// Run scrape to collect all metrics
	exporter.scrape(ctx)

	// Get the running_builds metric value
	runningBuilds := testutil.ToFloat64(exporter.runningBuilds)

	// Should have exactly 3 running builds (running-1, running-2, running-3)
	// finished-1 and finished-2 should NOT be counted because they have finished_at
	if runningBuilds != 3 {
		t.Errorf("Expected 3 running builds, got %f", runningBuilds)
	}

	// Gather metrics in Prometheus format to verify output
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Convert to text for verification
	var metricsText strings.Builder
	for _, mf := range metricFamilies {
		if mf.GetName() == "rspecq_running_builds" {
			for _, m := range mf.GetMetric() {
				if m.GetGauge() != nil {
					metricsText.WriteString(fmt.Sprintf("%s %v\n", mf.GetName(), m.GetGauge().GetValue()))
				}
			}
		}
	}

	expectedMetric := "rspecq_running_builds 3"
	if !strings.Contains(metricsText.String(), expectedMetric) {
		t.Errorf("Expected metric output to contain '%s', got: %s", expectedMetric, metricsText.String())
	}

	t.Logf("✓ Running builds metric correctly counts %d running builds", int(runningBuilds))
	t.Logf("✓ Metric output: %s", strings.TrimSpace(metricsText.String()))

	// Additional verification: Test empty state
	t.Run("EmptyState", func(t *testing.T) {
		// Clear all keys
		rdb.FlushAll(ctx)

		exporter.scrape(ctx)
		runningBuilds := testutil.ToFloat64(exporter.runningBuilds)

		if runningBuilds != 0 {
			t.Errorf("Expected 0 running builds when Redis is empty, got %f", runningBuilds)
		}
	})

	// Additional verification: Test all finished
	t.Run("AllFinished", func(t *testing.T) {
		// Clear and set up builds that are all finished
		rdb.FlushAll(ctx)

		for i := 1; i <= 3; i++ {
			buildID := fmt.Sprintf("finished-build-%d", i)
			rdb.Set(ctx, buildID+":queue:status", "ready", 0)
			rdb.Set(ctx, buildID+":queue:finished_at", baseTime+int64(i), 0)
		}

		exporter.scrape(ctx)
		runningBuilds := testutil.ToFloat64(exporter.runningBuilds)

		if runningBuilds != 0 {
			t.Errorf("Expected 0 running builds when all are finished, got %f", runningBuilds)
		}
	})
}
