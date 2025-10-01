# Build ID Regex Feature - Implementation Summary

## Overview
Added the ability to pass a command-line flag `--build-id-regex` that allows parsing build IDs with named regex capture groups. Extracted labels are automatically added to all build-level metrics.

## Changes Made

### 1. Core Implementation (`exporter.go`)
- Added `buildIDRegex *regexp.Regexp` and `labelNames []string` fields to `RSpecQExporter` struct
- Modified `NewRSpecQExporter()` to:
  - Accept a `buildIDRegexPattern string` parameter
  - Compile and validate the regex pattern
  - Extract label names from named capture groups
  - Return an error if regex is invalid
- Added `extractLabels(buildID string)` method to extract labels from build IDs using the regex
- Updated all metric label definitions to use dynamic `labelNames` instead of hardcoded `[]string{"build_id"}`
- Changed all `WithLabelValues(buildID)` calls to use `With(labels)` with prometheus.Labels map
- Special handling for `buildStatus` metric which has an additional "status" label

### 2. Main Entry Point (`main.go`)
- Added `--build-id-regex` command-line flag
- Updated exporter initialization to pass regex pattern and handle errors
- Added descriptive help text with example regex pattern

### 3. Tests (`regex_test.go`, `regex_integration_test.go`)
- Created comprehensive unit tests for regex extraction:
  - Testing with no regex (default behavior)
  - Testing with various regex patterns
  - Testing when regex doesn't match
  - Testing invalid regex patterns
  - Testing label name initialization
- Created end-to-end integration tests:
  - Testing full workflow with actual Redis instance
  - Testing multiple builds with different patterns
  - Testing non-matching build IDs

### 4. Test Updates (`exporter_test.go`)
- Updated all test calls to `NewRSpecQExporter()` to include the new parameter
- Added error handling for all test cases

### 5. Documentation
- Created `BUILD_ID_REGEX.md` with comprehensive documentation:
  - Usage examples
  - Multiple regex pattern examples
  - Prometheus query examples
  - Important notes and best practices
  - Regex syntax reference
- Updated `README.md`:
  - Added `--build-id-regex` flag to options table
  - Added example usage with regex
  - Added new "Advanced Features" section
  - Referenced detailed documentation
- Created `examples/run-with-regex.sh` with practical examples

## Key Features

### 1. Named Capture Groups
Any named group in the regex (e.g., `(?P<name>...)`) becomes a label on metrics:
```regex
(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)
```
Applied to build ID `myapp-main-12345` creates labels:
- `build_id`: `myapp-main-12345`
- `project`: `myapp`
- `branch`: `main`
- `build`: `12345`

### 2. Backward Compatible
- When no regex is provided, behavior is identical to before (only `build_id` label)
- Existing deployments are not affected

### 3. Error Handling
- Invalid regex patterns cause startup failure with clear error message
- Non-matching build IDs gracefully fall back to only `build_id` label

### 4. Dynamic Label Configuration
- Metrics are configured at startup based on regex pattern
- All build-level metrics automatically get the additional labels

## Usage Examples

### Example 1: Project/Branch/Build Pattern
```bash
./rspecq-exporter \
  --redis-addr=localhost:6379 \
  --build-id-regex='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'
```

### Example 2: Environment/Service Pattern
```bash
./rspecq-exporter \
  --redis-addr=localhost:6379 \
  --build-id-regex='(?P<env>[^-]+)-(?P<service>[^-]+)-(?P<version>.+)'
```

## Metrics Impact

All build-level metrics now support dynamic labels:
- `rspecq_build_queue_unprocessed`
- `rspecq_build_queue_running`
- `rspecq_build_queue_processed`
- `rspecq_build_queue_lost`
- `rspecq_build_example_count`
- `rspecq_build_example_failures`
- `rspecq_build_non_example_errors`
- `rspecq_build_requeues`
- `rspecq_build_status` (has additional "status" label)
- `rspecq_build_fail_fast`
- `rspecq_build_withdrawn_workers_count`
- `rspecq_worker_count`
- `rspecq_build_elected_master_at`
- `rspecq_build_ready_at`
- `rspecq_build_finished_at`
- `rspecq_build_duration_seconds`

## Prometheus Query Examples

With extracted labels, you can now create powerful queries:

```promql
# Filter by project
rspecq_build_queue_unprocessed{project="myapp"}

# Aggregate by branch
sum(rspecq_build_example_count) by (branch)

# Filter by environment
rspecq_build_duration_seconds{env="production"}

# Complex filtering
rspecq_build_example_failures{project="api",branch="main"}
```

## Testing

All tests pass:
```bash
$ go test ./...
ok      github.com/yourusername/rspecq-exporter 0.385s
```

Test coverage includes:
- Unit tests for regex extraction
- Unit tests for label initialization
- Integration tests with actual Redis
- Backward compatibility tests
- Error handling tests

## Files Modified
- `main.go` - Added flag and error handling
- `exporter.go` - Core implementation
- `exporter_test.go` - Updated existing tests

## Files Created
- `regex_test.go` - Unit tests for regex functionality
- `regex_integration_test.go` - E2E integration tests
- `BUILD_ID_REGEX.md` - Comprehensive documentation
- `examples/run-with-regex.sh` - Usage examples

## Backward Compatibility
✅ Fully backward compatible - no breaking changes
✅ Default behavior unchanged when flag is not provided
✅ Existing deployments work without modifications
