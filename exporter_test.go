package main

import (
	"testing"
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
)

func TestNewRSpecQExporter(t *testing.T) {
	// Create a mock Redis client (won't actually connect)
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	
	exporter := NewRSpecQExporter(rdb)
	
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
	
	exporter := NewRSpecQExporter(rdb)
	
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

// Note: Integration tests that actually connect to Redis should be in a separate file
// and perhaps skipped in unit test runs unless a test Redis is available.
