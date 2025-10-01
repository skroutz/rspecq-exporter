# Environment Variables Quick Reference

## All Available Environment Variables

```bash
# Redis Connection
export REDIS_ADDR="localhost:6379"           # or REDIS_ADDRESS
export REDIS_PASSWORD="your-password"
export REDIS_DB="0"                          # or REDIS_DATABASE

# Server Configuration
export LISTEN_ADDR=":9292"                   # or LISTEN_ADDRESS
export SCRAPE_INTERVAL="15s"

# Feature Flags
export DISABLE_PER_WORKER_METRICS="false"    # true to disable
export BUILD_ID_REGEX='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'
```

## Common Patterns

### Development
```bash
export REDIS_ADDR=localhost:6379
./rspecq-exporter
```

### Production
```bash
export REDIS_ADDR=redis.prod.internal:6379
export REDIS_PASSWORD=secretpassword
export SCRAPE_INTERVAL=30s
./rspecq-exporter
```

### Docker
```bash
docker run \
  -e REDIS_ADDR=redis:6379 \
  -e REDIS_PASSWORD=secret \
  -p 9292:9292 \
  rspecq-exporter
```

### Kubernetes Deployment
```yaml
spec:
  containers:
  - name: exporter
    image: rspecq-exporter:latest
    env:
    - name: REDIS_ADDR
      value: "redis.default.svc.cluster.local:6379"
    - name: REDIS_PASSWORD
      valueFrom:
        secretKeyRef:
          name: redis-secret
          key: password
```

## Tips

1. **Security**: Use environment variables for sensitive data like `REDIS_PASSWORD`
2. **Precedence**: Command-line flags override environment variables
3. **Multiple Names**: Some variables have aliases (e.g., `REDIS_ADDR` or `REDIS_ADDRESS`)
4. **Booleans**: Use `true`/`false` or `1`/`0` for boolean values
5. **Durations**: Use Go duration format (`15s`, `1m`, `1h30m`)

## Verification

Check which values are active:
```bash
./rspecq-exporter --help
```

The help output shows `[$VARIABLE_NAME]` after each option to indicate which environment variable it reads from.
