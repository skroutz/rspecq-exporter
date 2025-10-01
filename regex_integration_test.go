package main

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// TestE2E_BuildIDRegexLabels tests that build ID regex extraction works end-to-end
func TestE2E_BuildIDRegexLabels(t *testing.T) {
	// Start mini Redis server
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	ctx := context.Background()

	// Setup test data with a build ID that matches the regex pattern
	buildID := "myapp-main-12345"

	// Create build status
	_ = mr.Set(buildID+":queue:status", "ready")

	// Set build metrics
	_, _ = mr.Lpush(buildID+":queue:unprocessed", "job1")
	_, _ = mr.Lpush(buildID+":queue:unprocessed", "job2")
	_, _ = mr.Lpush(buildID+":queue:unprocessed", "job3")
	_ = mr.Set(buildID+":example_count", "100")
	mr.HSet(buildID+":example_failures", "spec1", "1")

	// Create exporter with regex
	regex := `(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)`
	exporter, err := NewRSpecQExporter(rdb, false, regex)
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Run scrape
	exporter.scrape(ctx)

	// Test that extracted labels are present
	labels := exporter.extractLabels(buildID)

	// Verify all expected labels are present
	expectedLabels := map[string]string{
		"build_id": "myapp-main-12345",
		"project":  "myapp",
		"branch":   "main",
		"build":    "12345",
	}

	for key, expectedValue := range expectedLabels {
		if actualValue, ok := labels[key]; !ok {
			t.Errorf("Missing label: %s", key)
		} else if actualValue != expectedValue {
			t.Errorf("Label %s: got %q, want %q", key, actualValue, expectedValue)
		}
	}

	// Verify label names in exporter
	expectedLabelNames := []string{"build_id", "project", "branch", "build"}
	if len(exporter.labelNames) != len(expectedLabelNames) {
		t.Errorf("Label names count: got %d, want %d", len(exporter.labelNames), len(expectedLabelNames))
	}

	t.Logf("✓ Build ID regex extraction working correctly")
	t.Logf("✓ Extracted labels: %v", labels)
	t.Logf("✓ Exporter label names: %v", exporter.labelNames)
}

// TestE2E_BuildIDRegexNoMatch tests behavior when regex doesn't match
func TestE2E_BuildIDRegexNoMatch(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	// Create exporter with regex that won't match
	regex := `(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)`
	exporter, err := NewRSpecQExporter(rdb, false, regex)
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Test with a build ID that doesn't match
	buildID := "nomatch"
	labels := exporter.extractLabels(buildID)

	// Should only have build_id label
	if len(labels) != 1 {
		t.Errorf("Expected only build_id label, got %d labels: %v", len(labels), labels)
	}

	if labels["build_id"] != buildID {
		t.Errorf("build_id label: got %q, want %q", labels["build_id"], buildID)
	}

	t.Logf("✓ Non-matching build ID handled correctly")
	t.Logf("✓ Labels: %v", labels)
}

// TestE2E_BuildIDRegexMultipleBuilds tests regex extraction with multiple builds
func TestE2E_BuildIDRegexMultipleBuilds(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	ctx := context.Background()

	// Setup multiple builds with different patterns
	builds := []string{
		"frontend-develop-100",
		"backend-main-200",
		"api-staging-300",
	}

	for _, buildID := range builds {
		_ = mr.Set(buildID+":queue:status", "ready")
		_ = mr.Set(buildID+":example_count", "50")
	}

	// Create exporter with regex
	regex := `(?P<project>[\w-]+)-(?P<branch>[\w-]+)-(?P<build>\d+)`
	exporter, err := NewRSpecQExporter(rdb, false, regex)
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	// Run scrape
	exporter.scrape(ctx)

	// Verify each build gets proper labels
	testCases := []struct {
		buildID string
		project string
		branch  string
		build   string
	}{
		{"frontend-develop-100", "frontend", "develop", "100"},
		{"backend-main-200", "backend", "main", "200"},
		{"api-staging-300", "api", "staging", "300"},
	}

	for _, tc := range testCases {
		labels := exporter.extractLabels(tc.buildID)

		if labels["build_id"] != tc.buildID {
			t.Errorf("Build %s: build_id label incorrect", tc.buildID)
		}
		if labels["project"] != tc.project {
			t.Errorf("Build %s: project label got %q, want %q", tc.buildID, labels["project"], tc.project)
		}
		if labels["branch"] != tc.branch {
			t.Errorf("Build %s: branch label got %q, want %q", tc.buildID, labels["branch"], tc.branch)
		}
		if labels["build"] != tc.build {
			t.Errorf("Build %s: build label got %q, want %q", tc.buildID, labels["build"], tc.build)
		}
	}

	t.Logf("✓ Multiple builds with regex extraction working correctly")
}
