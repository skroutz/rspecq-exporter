# Configuration Guide

The RSpecQ exporter can be configured using either command-line flags or environment variables. Environment variables take precedence when both are provided.

## Configuration Options

### Redis Connection

| Flag | Environment Variables | Default | Description |
|------|---------------------|---------|-------------|
| `--redis-addr` | `REDIS_ADDR`, `REDIS_ADDRESS` | `localhost:6379` | Redis server address |
| `--redis-password` | `REDIS_PASSWORD` | _(empty)_ | Redis password for authentication |
| `--redis-db` | `REDIS_DB`, `REDIS_DATABASE` | `0` | Redis database number |

### Server Settings

| Flag | Environment Variables | Default | Description |
|------|---------------------|---------|-------------|
| `--listen-addr` | `LISTEN_ADDR`, `LISTEN_ADDRESS` | `:9292` | Address to listen on for metrics endpoint |
| `--scrape-interval` | `SCRAPE_INTERVAL` | `15s` | Interval for scraping Redis metrics |

### Metrics Configuration

| Flag | Environment Variables | Default | Description |
|------|---------------------|---------|-------------|
| `--disable-per-worker-metrics` | `DISABLE_PER_WORKER_METRICS` | `false` | Disable per-worker metrics (reduces cardinality) |
| `--build-id-regex` | `BUILD_ID_REGEX` | _(empty)_ | Named capture group regex to extract labels from build IDs |

## Usage Examples

### Using Command-Line Flags

```bash
./rspecq-exporter \
  --redis-addr=localhost:6379 \
  --redis-password=secret \
  --listen-addr=:9292 \
  --scrape-interval=30s \
  --build-id-regex='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'
```

### Using Environment Variables

```bash
export REDIS_ADDR=localhost:6379
export REDIS_PASSWORD=secret
export LISTEN_ADDR=:9292
export SCRAPE_INTERVAL=30s
export BUILD_ID_REGEX='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'

./rspecq-exporter
```

### Mixed Configuration

You can mix flags and environment variables. Flags take precedence over environment variables:

```bash
export REDIS_ADDR=localhost:6379
export REDIS_PASSWORD=secret

# Override REDIS_ADDR with flag
./rspecq-exporter --redis-addr=redis.example.com:6379
```

### Docker Usage

```bash
docker run -e REDIS_ADDR=redis:6379 \
           -e REDIS_PASSWORD=secret \
           -e LISTEN_ADDR=:9292 \
           -e BUILD_ID_REGEX='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)' \
           -p 9292:9292 \
           rspecq-exporter:latest
```

### Kubernetes ConfigMap/Secret

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rspecq-exporter-config
data:
  REDIS_ADDR: "redis.default.svc.cluster.local:6379"
  LISTEN_ADDR: ":9292"
  SCRAPE_INTERVAL: "15s"
  BUILD_ID_REGEX: '(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'
---
apiVersion: v1
kind: Secret
metadata:
  name: rspecq-exporter-secret
type: Opaque
stringData:
  REDIS_PASSWORD: "your-secret-password"
```

Then reference in your deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rspecq-exporter
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rspecq-exporter
  template:
    metadata:
      labels:
        app: rspecq-exporter
    spec:
      containers:
      - name: exporter
        image: rspecq-exporter:latest
        ports:
        - containerPort: 9292
        envFrom:
        - configMapRef:
            name: rspecq-exporter-config
        - secretRef:
            name: rspecq-exporter-secret
```

## Help

To see all available options:

```bash
./rspecq-exporter --help
```

Output:

```text
NAME:
   rspecq-exporter - Prometheus exporter for RSpecQ metrics from Redis

USAGE:
   rspecq-exporter [global options] command [command options] [arguments...]

GLOBAL OPTIONS:
   --redis-addr value            Redis address (default: "localhost:6379") [$REDIS_ADDR, $REDIS_ADDRESS]
   --redis-password value        Redis password [$REDIS_PASSWORD]
   --redis-db value              Redis database number (default: 0) [$REDIS_DB, $REDIS_DATABASE]
   --listen-addr value           Address to listen on for metrics (default: ":9292") [$LISTEN_ADDR, $LISTEN_ADDRESS]
   --scrape-interval value       Interval for scraping Redis metrics (default: 15s) [$SCRAPE_INTERVAL]
   --disable-per-worker-metrics  Disable metrics about individual workers (reduces cardinality) (default: false) [$DISABLE_PER_WORKER_METRICS]
   --build-id-regex value        Named capture group regex to extract labels from build IDs (e.g., '(?P<project>\w+)-(?P<branch>\w+)-(?P<build>\d+)') [$BUILD_ID_REGEX]
   --help, -h                    show help
```

## Duration Format

The `--scrape-interval` / `SCRAPE_INTERVAL` option accepts Go duration strings:

- `30s` - 30 seconds
- `1m` - 1 minute
- `1m30s` - 1 minute 30 seconds
- `1h` - 1 hour

## Boolean Flags

For boolean flags like `--disable-per-worker-metrics`, you can:

- Use the flag: `--disable-per-worker-metrics` (sets to true)
- Set environment variable: `DISABLE_PER_WORKER_METRICS=true` or `DISABLE_PER_WORKER_METRICS=1`
