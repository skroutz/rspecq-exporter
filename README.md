# RSpecQ Prometheus Exporter

A Prometheus exporter for [RSpecQ](https://github.com/skroutz/rspecq), a distributed test runner system for Ruby projects. This exporter connects to the Redis instance used by RSpecQ to collect and expose metrics about test builds, workers, and queue states.

## Overview

RSpecQ uses Redis as its synchronization backend, storing all build state, worker information, and job queues in Redis data structures. This exporter reads from that Redis instance to provide observability into your distributed test infrastructure.

## Features

- **Build Metrics**: Track queue sizes, processed jobs, failures, and build status
- **Worker Metrics**: Monitor active workers, heartbeats, and abnormal terminations
- **Timing Metrics**: Measure build durations and track timing data
- **Global Metrics**: View historical timing data and build statistics
- **Auto-discovery**: Automatically discovers active builds from Redis
- **Prometheus Native**: Exports metrics in Prometheus format

## Installation

### From Source

```bash
go build -o rspecq-exporter
```

### Using Docker

```bash
docker build -t rspecq-exporter .
docker run -p 9292:9292 rspecq-exporter --redis-addr=redis:6379
```

## Usage

```bash
./rspecq-exporter [options]
```

### Command-line Options

All options can be configured using either command-line flags or environment variables. Environment variables provide an alternative to flags, especially useful in containerized environments.

| Flag | Environment Variables | Default | Description |
|------|---------------------|---------|-------------|
| `--redis-addr` | `REDIS_ADDR`, `REDIS_ADDRESS` | `localhost:6379` | Redis server address |
| `--redis-password` | `REDIS_PASSWORD` | `""` | Redis password (if required) |
| `--redis-db` | `REDIS_DB`, `REDIS_DATABASE` | `0` | Redis database number |
| `--listen-addr` | `LISTEN_ADDR`, `LISTEN_ADDRESS` | `:9292` | HTTP server listen address |
| `--scrape-interval` | `SCRAPE_INTERVAL` | `15s` | Interval for scraping Redis metrics |
| `--disable-per-worker-metrics` | `DISABLE_PER_WORKER_METRICS` | `false` | Disable metrics about individual workers (reduces cardinality) |
| `--build-id-regex` | `BUILD_ID_REGEX` | `""` | Named capture group regex to extract labels from build IDs |

**For detailed configuration documentation, see [CONFIGURATION.md](CONFIGURATION.md).**

### Example

```bash
# Basic usage with flags
./rspecq-exporter --redis-addr=localhost:6379

# Using environment variables
export REDIS_ADDR=redis.example.com:6379
export REDIS_PASSWORD=secret
export LISTEN_ADDR=:8080
export SCRAPE_INTERVAL=10s
./rspecq-exporter

# With authentication and custom port (flags)
./rspecq-exporter \
  --redis-addr=redis.example.com:6379 \
  --redis-password=secret \
  --listen-addr=:8080 \
  --scrape-interval=10s

# With build ID regex to extract custom labels
./rspecq-exporter \
  --redis-addr=localhost:6379 \
  --build-id-regex='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'

# Disable per-worker metrics to reduce cardinality
./rspecq-exporter \
  --redis-addr=localhost:6379 \
  --disable-per-worker-metrics

# Docker with environment variables
docker run -p 9292:9292 \
  -e REDIS_ADDR=redis:6379 \
  -e REDIS_PASSWORD=secret \
  -e BUILD_ID_REGEX='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)' \
  rspecq-exporter
```

## Advanced Features

### Build ID Label Extraction

The `--build-id-regex` flag allows you to parse build IDs using named capture groups and extract custom labels. This enables more powerful Prometheus queries and aggregations.

For example, if your build IDs follow the pattern `my-project-main-12345`, you can use:

```bash
./rspecq-exporter --build-id-regex='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'
```

This will add `project`, `branch`, and `build` labels to all build-level metrics, in addition to the standard `build_id` label.

**See [BUILD_ID_REGEX.md](BUILD_ID_REGEX.md) for detailed documentation and examples.**
  --scrape-interval=10s

# Disable per-worker metrics to reduce cardinality
./rspecq-exporter \
  --redis-addr=localhost:6379 \
  --disable-per-worker-metrics
```

## Metrics

### Build-Level Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rspecq_build_queue_unprocessed` | Gauge | `build_id` | Number of jobs waiting to be processed |
| `rspecq_build_queue_running` | Gauge | `build_id` | Number of jobs currently being executed |
| `rspecq_build_queue_processed` | Gauge | `build_id` | Number of completed jobs |
| `rspecq_build_queue_lost` | Gauge | `build_id` | Number of jobs lost due to worker failures |
| `rspecq_build_example_count` | Gauge | `build_id` | Total number of RSpec examples executed |
| `rspecq_build_example_failures` | Gauge | `build_id` | Number of failed examples |
| `rspecq_build_non_example_errors` | Gauge | `build_id` | Number of non-example errors (e.g., syntax errors) |
| `rspecq_build_requeues` | Gauge | `build_id` | Number of jobs that were requeued |
| `rspecq_build_status` | Gauge | `build_id`, `status` | Build status (1 = active for that status, 0 = inactive). Status values: `initializing`, `ready` |
| `rspecq_build_fail_fast` | Gauge | `build_id` | Fail-fast threshold (0 = disabled) |

### Worker Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rspecq_worker_count` | Gauge | `build_id` | Number of active workers for a build (aggregate metric) |
| `rspecq_build_withdrawn_workers_count` | Gauge | `build_id` | Total number of withdrawn workers for a build (aggregate metric) |
| `rspecq_worker_heartbeat_timestamp` | Gauge | `build_id`, `worker_id` | Unix timestamp of last worker heartbeat (per-worker metric, disabled with `--disable-per-worker-metrics`) |
| `rspecq_workers_withdrawn` | Gauge | `build_id`, `worker_id` | Count of abnormal worker terminations (per-worker metric, disabled with `--disable-per-worker-metrics`) |

> **Note**: The `--disable-per-worker-metrics` flag disables only the metrics with the `worker_id` label (`rspecq_worker_heartbeat_timestamp` and `rspecq_workers_withdrawn`), which can lead to high cardinality in environments with many workers. The aggregate metrics `rspecq_worker_count` and `rspecq_build_withdrawn_workers_count` remain enabled as they only have the `build_id` label.

### Timing Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rspecq_build_elected_master_at` | Gauge | `build_id` | Unix timestamp when master worker was elected |
| `rspecq_build_ready_at` | Gauge | `build_id` | Unix timestamp when queue became ready |
| `rspecq_build_duration_seconds` | Gauge | `build_id` | Build duration in seconds |

### Global Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rspecq_global_timings_count` | Gauge | - | Number of entries in global timings database |
| `rspecq_build_times_count` | Gauge | - | Number of historical build times stored |

### Exporter Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rspecq_scrape_success` | Gauge | - | Whether the last scrape was successful (1 = success) |
| `rspecq_scrape_duration_seconds` | Gauge | - | Duration of last scrape in seconds |
| `rspecq_last_scrape_timestamp` | Gauge | - | Unix timestamp of last scrape |

## Prometheus Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'rspecq'
    static_configs:
      - targets: ['localhost:9292']
    scrape_interval: 15s
```

## Example Queries

### Active Builds
```promql
count(rspecq_build_queue_unprocessed > 0)
```

### Build Progress (Percentage Complete)
```promql
100 * rspecq_build_queue_processed /
  (rspecq_build_queue_processed + rspecq_build_queue_unprocessed + rspecq_build_queue_running)
```

### Worker Failure Rate
```promql
rate(rspecq_build_queue_lost[5m])
```

### Average Build Duration
```promql
avg(rspecq_build_duration_seconds)
```

### Builds with Failures
```promql
rspecq_build_example_failures > 0
```

### Stale Workers (No heartbeat in 60s)
```promql
time() - rspecq_worker_heartbeat_timestamp > 60
```

## Grafana Dashboard

A sample Grafana dashboard is available in `grafana/dashboard.json` (to be created). Import it to get started quickly.

### Key Panels
- Build queue sizes over time
- Active worker count
- Build success/failure rate
- Build duration histogram
- Worker heartbeat status

## Architecture

### RSpecQ Redis Data Structure

RSpecQ stores data in Redis with the following key patterns:

- `<build_id>:queue:unprocessed` - LIST of pending jobs
- `<build_id>:queue:running` - HASH of jobs currently being executed
- `<build_id>:queue:processed` - SET of completed jobs
- `<build_id>:worker_heartbeats` - ZSET of worker heartbeats (score = timestamp)
- `<build_id>:example_failures` - HASH of failed examples
- `timings` - ZSET of global timing data for test scheduling

The exporter scans Redis for active builds and collects metrics from these data structures.

## Development

### Prerequisites
- Go 1.21 or later
- Redis (for testing)
- RSpecQ (for generating test data)

### Running Locally

```bash
# Install dependencies
go mod download

# Run the exporter
go run . --redis-addr=localhost:6379

# Run tests
go test ./...

# Build
go build -o rspecq-exporter
```

### Testing with RSpecQ

1. Start Redis:
   ```bash
   docker run -d -p 6379:6379 redis:7-alpine
   ```

2. Run an RSpecQ build:
   ```bash
   # In your Ruby project
   bundle exec rspecq --build mybuild --worker worker1
   ```

3. Start the exporter:
   ```bash
   ./rspecq-exporter --redis-addr=localhost:6379
   ```

4. View metrics:
   ```bash
   curl http://localhost:9292/metrics
   ```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Areas for Contribution
- Additional metrics
- Performance optimizations
- Documentation improvements
- Example dashboards
- Integration tests

## License

MIT License - see LICENSE file for details

## Related Projects

- [RSpecQ](https://github.com/skroutz/rspecq) - The distributed RSpec test runner
- [Prometheus](https://prometheus.io/) - Monitoring and alerting toolkit
- [Grafana](https://grafana.com/) - Visualization platform

## Support

For issues and questions:
- Open an issue on GitHub
- Check RSpecQ documentation for Redis schema details
- Consult Prometheus best practices for metric design
