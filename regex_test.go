package main

import (
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
)

func TestBuildIDRegexExtraction(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	tests := []struct {
		name        string
		regex       string
		buildID     string
		wantLabels  prometheus.Labels
		expectError bool
	}{
		{
			name:    "no regex - only build_id label",
			regex:   "",
			buildID: "my-project-main-12345",
			wantLabels: prometheus.Labels{
				"build_id": "my-project-main-12345",
			},
			expectError: false,
		},
		{
			name:    "extract project, branch, and build number",
			regex:   `(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)`,
			buildID: "my-project-main-12345",
			wantLabels: prometheus.Labels{
				"build_id": "my-project-main-12345",
				"project":  "my-project",
				"branch":   "main",
				"build":    "12345",
			},
			expectError: false,
		},
		{
			name:    "extract with different format",
			regex:   `(?P<env>[^-]+)-(?P<service>[^-]+)-(?P<version>.+)`,
			buildID: "production-api-v1.2.3",
			wantLabels: prometheus.Labels{
				"build_id": "production-api-v1.2.3",
				"env":      "production",
				"service":  "api",
				"version":  "v1.2.3",
			},
			expectError: false,
		},
		{
			name:    "regex doesn't match - fallback to build_id only",
			regex:   `(?P<project>\w+)-(?P<branch>\w+)-(?P<build>\d+)`,
			buildID: "nomatch",
			wantLabels: prometheus.Labels{
				"build_id": "nomatch",
			},
			expectError: false,
		},
		{
			name:        "invalid regex",
			regex:       `(?P<invalid`,
			buildID:     "test",
			wantLabels:  nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, err := NewRSpecQExporter(rdb, false, tt.regex)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			labels := exporter.extractLabels(tt.buildID)

			if len(labels) != len(tt.wantLabels) {
				t.Errorf("Label count mismatch: got %d, want %d", len(labels), len(tt.wantLabels))
			}

			for key, wantValue := range tt.wantLabels {
				gotValue, ok := labels[key]
				if !ok {
					t.Errorf("Missing label: %s", key)
					continue
				}
				if gotValue != wantValue {
					t.Errorf("Label %s: got %q, want %q", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestLabelNamesInitialization(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	tests := []struct {
		name           string
		regex          string
		expectedLabels []string
	}{
		{
			name:           "no regex",
			regex:          "",
			expectedLabels: []string{"build_id"},
		},
		{
			name:           "with named groups",
			regex:          `(?P<project>\w+)-(?P<branch>\w+)-(?P<build>\d+)`,
			expectedLabels: []string{"build_id", "project", "branch", "build"},
		},
		{
			name:           "single named group",
			regex:          `(?P<env>\w+)-.+`,
			expectedLabels: []string{"build_id", "env"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, err := NewRSpecQExporter(rdb, false, tt.regex)
			if err != nil {
				t.Fatalf("Failed to create exporter: %v", err)
			}

			if len(exporter.labelNames) != len(tt.expectedLabels) {
				t.Errorf("Label names count: got %d, want %d", len(exporter.labelNames), len(tt.expectedLabels))
			}

			labelMap := make(map[string]bool)
			for _, label := range exporter.labelNames {
				labelMap[label] = true
			}

			for _, expected := range tt.expectedLabels {
				if !labelMap[expected] {
					t.Errorf("Missing expected label: %s", expected)
				}
			}
		})
	}
}
