# Quick Start Guide - RSpecQ Exporter

This guide will help you get the RSpecQ Prometheus Exporter up and running quickly.

## Prerequisites

- Go 1.21+ (for building from source)
- Docker (optional, for containerized deployment)
- Access to a Redis instance used by RSpecQ

## Quick Start Options

### Option 1: Run with Docker Compose (Easiest)

This method sets up everything: Redis, RSpecQ Exporter, Prometheus, and Grafana.

```bash
# Clone the repository
git clone https://github.com/yourusername/rspecq-exporter.git
cd rspecq-exporter

# Start all services
docker-compose up -d

# Access the services
# - Exporter metrics: http://localhost:9292/metrics
# - Prometheus: http://localhost:9090
# - Grafana: http://localhost:3000 (admin/admin)
```

### Option 2: Build and Run Locally

```bash
# Clone the repository
git clone https://github.com/yourusername/rspecq-exporter.git
cd rspecq-exporter

# Build
go build -o rspecq-exporter

# Run (assuming Redis is at localhost:6379)
./rspecq-exporter --redis-addr=localhost:6379

# View metrics
curl http://localhost:9292/metrics
```

### Option 3: Run with Docker

```bash
# Build the image
docker build -t rspecq-exporter .

# Run (replace with your Redis host)
docker run -p 9292:9292 rspecq-exporter --redis-addr=your-redis-host:6379

# View metrics
curl http://localhost:9292/metrics
```

## Connecting to Your RSpecQ Instance

The exporter needs to connect to the same Redis instance that RSpecQ uses.

```bash
# Basic connection
./rspecq-exporter --redis-addr=redis.example.com:6379

# With password
./rspecq-exporter \
  --redis-addr=redis.example.com:6379 \
  --redis-password=your-password

# With custom scrape interval
./rspecq-exporter \
  --redis-addr=redis.example.com:6379 \
  --scrape-interval=10s
```

## Testing with Sample Data

To see the exporter in action, you need RSpecQ to populate Redis with data.

### 1. Start Redis

```bash
docker run -d -p 6379:6379 --name redis redis:7-alpine
```

### 2. Run RSpecQ (in your Ruby project)

```bash
# Terminal 1: Start a worker
bundle exec rspecq --build test-build-1 --worker worker-1 spec/

# Terminal 2: Start another worker (optional)
bundle exec rspecq --build test-build-1 --worker worker-2 spec/

# Terminal 3: Run the reporter
bundle exec rspecq --build test-build-1 --report
```

### 3. Start the Exporter

```bash
# Terminal 4: Start the exporter
./rspecq-exporter --redis-addr=localhost:6379
```

### 4. View Metrics

```bash
# See all metrics
curl http://localhost:9292/metrics

# Filter for specific metrics
curl -s http://localhost:9292/metrics | grep rspecq_build_queue
```

## Integrating with Prometheus

### 1. Configure Prometheus

Add this to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'rspecq'
    static_configs:
      - targets: ['localhost:9292']
    scrape_interval: 15s
```

### 2. Reload Prometheus

```bash
# If using Docker
docker kill -s HUP prometheus

# If using systemd
sudo systemctl reload prometheus
```

### 3. Verify in Prometheus

Open Prometheus UI (typically http://localhost:9090) and search for `rspecq_` metrics.

## Example Queries

Once metrics are being collected, try these Prometheus queries:

### Build Progress
```promql
100 * rspecq_build_queue_processed{build_id="your-build-id"} / 
  (rspecq_build_queue_processed{build_id="your-build-id"} + 
   rspecq_build_queue_unprocessed{build_id="your-build-id"} + 
   rspecq_build_queue_running{build_id="your-build-id"})
```

### Active Workers
```promql
rspecq_worker_count{build_id="your-build-id"}
```

### Build Duration
```promql
rspecq_build_duration_seconds{build_id="your-build-id"}
```

### Failed Examples
```promql
rspecq_build_example_failures{build_id="your-build-id"}
```

## Grafana Dashboard

### Import Dashboard

1. Log in to Grafana (http://localhost:3000)
2. Click "+" → "Import"
3. Upload `grafana/dashboard.json` (when available)
4. Select your Prometheus data source
5. Click "Import"

### Create Basic Dashboard

1. Add a new panel
2. Select Prometheus as data source
3. Use one of the example queries above
4. Configure visualization (Graph, Gauge, Stat, etc.)
5. Save the dashboard

## Common Issues

### "Connection refused" when connecting to Redis

- Verify Redis is running: `redis-cli ping`
- Check the Redis host and port
- If using Docker, use `host.docker.internal` instead of `localhost`

### No metrics appear

- Ensure RSpecQ has run and populated Redis
- Check that the build IDs match
- View exporter logs for errors
- Verify Redis connection: `redis-cli -h your-host keys "*"`

### Metrics are stale

- Check the `--scrape-interval` setting
- Verify RSpecQ workers are actively running
- Look at `rspecq_last_scrape_timestamp` metric

## Next Steps

- Explore all available metrics in the [README](README.md#metrics)
- Set up alerts for build failures
- Create custom Grafana dashboards
- Configure production deployment
- Read the [Action Plan](ACTION_PLAN.md) for contributing

## Support

If you encounter issues:

1. Check the logs: `docker logs rspecq-exporter` or view console output
2. Verify Redis connectivity: `redis-cli -h your-host ping`
3. Check RSpecQ is writing data: `redis-cli -h your-host keys "*build*"`
4. Open an issue on GitHub with details

## Useful Commands

```bash
# Check if exporter is healthy
curl http://localhost:9292/

# View raw metrics
curl http://localhost:9292/metrics

# Filter specific build
curl -s http://localhost:9292/metrics | grep 'build_id="your-build"'

# Check scrape success
curl -s http://localhost:9292/metrics | grep rspecq_scrape_success

# View Redis keys (for debugging)
redis-cli keys "your-build-id:*"

# Follow exporter logs (if using Docker)
docker logs -f rspecq-exporter
```

## Production Considerations

Before deploying to production:

1. **Security**: Use Redis password authentication
2. **Monitoring**: Set up alerts on `rspecq_scrape_success`
3. **Resources**: Adjust scrape interval based on load
4. **High Availability**: Consider running multiple exporter instances
5. **Networking**: Ensure firewall rules allow Prometheus → Exporter traffic

Happy monitoring! 🚀
