# GitHub Copilot Instructions for RSpecQ Exporter Feature Development

## Project Overview
This is a Prometheus exporter for RSpecQ (distributed test runner). It connects to Redis to collect and expose metrics about test builds, workers, and queue states.

## Feature Development Guidelines

When implementing a new metric or feature, follow this structured approach:

### 1. Implementation (exporter.go)
- Add the new metric field to the `RSpecQExporter` struct
- Initialize the metric in `NewRSpecQExporter()` with proper help text and labels
- **IMPORTANT**: Add the metric to the appropriate metric collection (`buildMetrics`, `workerMetrics`, `timingMetrics`, `globalMetrics`, or `scrapeMetrics`) to ensure it's properly described, collected, and reset
- Implement data collection logic in the `scrape()` method or appropriate helper method
- For Lua scripts, define them at package level using `redis.NewScript()` for EVALSHA optimization
- Follow the existing patterns for Redis key naming: `<build_id>:<metric_name>`

### 2. E2E Testing (exporter_test.go)
- **Always update `TestE2E_HappyPath_AllMetrics` first** - this is the primary integration test
- Add test data setup in the test (e.g., `rdb.ZAdd(ctx, buildID+":metric_key", ...)`)
- Add metric validation to the `testCases` array with:
  - Prometheus format string (e.g., `` `rspecq_build_metric_name{build_id="...",label="..."}` ``)
  - Expected value
  - Getter function using `testutil.ToFloat64()`
- Test the happy path with realistic data

### 3. Edge Cases & Unit Tests (exporter_test.go or separate test file)
- Write **separate unit tests** for edge cases:
  - Missing or empty Redis data
  - Boundary conditions
- Name tests descriptively: `TestMetricName_EdgeCase` (e.g., `TestNextTestTiming_NoUnprocessedJobs`)
- Use `miniredis` for Redis mocking in integration tests
- Test error handling and fallback behavior

### 4. Documentation (README.md)
- Add the metric to the **Metrics** table with:
  - Metric name (e.g., `rspecq_build_metric_name`)
  - Type (Gauge, Counter, etc.)
  - Labels (e.g., `build_id`, custom labels)
  - Description (be clear and concise)
- Add a usage example in the **Prometheus Queries** section showing a practical query
- Document the Redis key pattern in the **Redis Data Structure** section with:
  - Key pattern (e.g., `<build_id>:metric_name`)
  - Redis data type (STRING, ZSET, HASH, LIST, etc.)
  - Brief description of what the key contains

### 5. Code Quality Standards
- Use meaningful variable names that match RSpecQ terminology
- Add comments for non-obvious logic, especially Lua scripts
- Handle Redis errors gracefully (log and continue)
- Maintain consistent metric naming: `rspecq_<category>_<metric>_<unit>`
- Use appropriate Prometheus metric types (Gauge for point-in-time values)
- Keep cardinality in mind - avoid unbounded label values

### 6. Commit Message
Follow this format:
```
Add <metric_name> metric

Brief description of what the metric exposes and why it's useful.

New metric:
  rspecq_build_metric_name{build_id="myapp-123",label="value"} 42.5

Changes:
- Add metricName GaugeVec with labels
- Collect data from <build_id>:redis_key Redis TYPE
- Add tests in TestE2E_HappyPath_AllMetrics
- Update README with metric docs and example query
```

## Testing Checklist
Before submitting a PR, ensure:
- [ ] `TestE2E_HappyPath_AllMetrics` passes with new metric validation
- [ ] Edge case tests cover: empty data
- [ ] All existing tests still pass (`go test -v ./...`)
- [ ] README documentation is complete (metric table + query example + Redis key)
- [ ] Metric is added to appropriate collection in `NewRSpecQExporter()`
- [ ] Commit message follows pattern: "Add <metric_name> metric" with detailed description

## Questions to Ask When Adding a Feature
1. What Redis key pattern does RSpecQ use for this data?
2. What's the Redis data type (STRING, ZSET, HASH, LIST)?
3. What labels are needed? (Always include `build_id`, consider others)
4. What's the appropriate Prometheus metric type?
5. What edge cases exist? (empty data)
6. How should this metric be queried in Prometheus?
7. Does this increase cardinality significantly?

## Additional Notes
- The exporter auto-discovers active builds via `*:queue:status` keys
- Use `miniredis` for fast, isolated Redis testing
- Metrics are scraped on an interval (default 15s)
- Worker metrics are disabled by default to reduce cardinality
