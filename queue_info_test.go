package main

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestQueueInfo_MissingInfo tests that missing info hash doesn't cause errors
func TestQueueInfo_MissingInfo(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	buildID := "test-build-missing-info"

	// Set up a build without info hash
	if err := rdb.Set(ctx, buildID+":queue:status", "ready", 0).Err(); err != nil {
		t.Fatalf("Failed to set status: %v", err)
	}

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Should not error when info is missing
	_, err = exporter.collectBuildMetrics(ctx, buildID)
	if err != nil {
		t.Errorf("Expected no error with missing info hash, got: %v", err)
	}
}

// TestQueueInfo_NonNumericStringValues tests that non-numeric string values are ignored gracefully
func TestQueueInfo_NonNumericStringValues(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	buildID := "test-build-string-values"

	// Set up a build with non-numeric string values
	if err := rdb.Set(ctx, buildID+":queue:status", "ready", 0).Err(); err != nil {
		t.Fatalf("Failed to set status: %v", err)
	}
	if err := rdb.HSet(ctx, buildID+":info", map[string]interface{}{
		"jobs":         "100",
		"description":  "this is a text description",
		"status":       "running",
		"untimed_jobs": "not-a-number",
		"valid_metric": "42",
	}).Err(); err != nil {
		t.Fatalf("Failed to set info hash: %v", err)
	}

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Should not error with non-numeric values
	_, err = exporter.collectBuildMetrics(ctx, buildID)
	if err != nil {
		t.Errorf("Expected no error with non-numeric string values, got: %v", err)
	}

	// Valid numeric strings should be parsed correctly
	if actual := testutil.ToFloat64(exporter.buildQueueInfo.WithLabelValues(buildID, "jobs")); actual != 100.0 {
		t.Errorf("Expected jobs to be 100.0, got %f", actual)
	}
	if actual := testutil.ToFloat64(exporter.buildQueueInfo.WithLabelValues(buildID, "valid_metric")); actual != 42.0 {
		t.Errorf("Expected valid_metric to be 42.0, got %f", actual)
	}

	// Non-numeric strings should be exposed via buildQueueInfoStrings
	if actual := testutil.ToFloat64(exporter.buildQueueInfoStrings.WithLabelValues(buildID, "description", "this is a text description")); actual != 1.0 {
		t.Errorf("Expected description metric to be 1.0, got %f", actual)
	}
	if actual := testutil.ToFloat64(exporter.buildQueueInfoStrings.WithLabelValues(buildID, "status", "running")); actual != 1.0 {
		t.Errorf("Expected status metric to be 1.0, got %f", actual)
	}
	if actual := testutil.ToFloat64(exporter.buildQueueInfoStrings.WithLabelValues(buildID, "untimed_jobs", "not-a-number")); actual != 1.0 {
		t.Errorf("Expected untimed_jobs string metric to be 1.0, got %f", actual)
	}
}

// TestQueueInfo_DynamicDiscovery tests that any hash key is collected dynamically
func TestQueueInfo_DynamicDiscovery(t *testing.T) {
	rdb, _, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	buildID := "test-build-dynamic-info"

	// Set up a build with custom/future stat names
	if err := rdb.Set(ctx, buildID+":queue:status", "ready", 0).Err(); err != nil {
		t.Fatalf("Failed to set status: %v", err)
	}
	if err := rdb.HSet(ctx, buildID+":info", map[string]interface{}{
		"jobs":           "80",
		"custom_stat":    "42",
		"files_splitted": "2",
	}).Err(); err != nil {
		t.Fatalf("Failed to set info hash: %v", err)
	}

	exporter, err := NewRSpecQExporter(rdb, false, "")
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Collect metrics
	_, err = exporter.collectBuildMetrics(ctx, buildID)
	if err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	// Verify all stats are dynamically discovered and collected
	tests := []struct {
		stat     string
		expected float64
	}{
		{"jobs", 80.0},
		{"custom_stat", 42.0},
		{"files_splitted", 2.0},
	}

	for _, tc := range tests {
		actual := testutil.ToFloat64(exporter.buildQueueInfo.WithLabelValues(buildID, tc.stat))
		if actual != tc.expected {
			t.Errorf("Expected %s to be %f, got %f", tc.stat, tc.expected, actual)
		}
	}
}
