# Build Discovery Testing Example

## How the Tests Work

The tests write actual data to a Redis instance (using miniredis for in-memory testing) and verify that the exporter correctly discovers and processes RSpecQ builds.

## Example Test Flow

### 1. Test Setup
```go
// Create an in-memory Redis instance
rdb, _, cleanup := setupTestRedis(t)
defer cleanup()

ctx := context.Background()
buildID := "test-build-123"
```

### 2. Write Data to Redis
```go
// Create a status key - this is what the exporter scans for
err := rdb.Set(ctx, buildID+":status", "ready", 0).Err()
if err != nil {
    t.Fatalf("Failed to create status key: %v", err)
}
```

### 3. Test Build Discovery
```go
// Create exporter and discover builds
exporter := NewRSpecQExporter(rdb)
builds, err := exporter.discoverBuilds(ctx)

// Verify the build was discovered
if len(builds) != 1 {
    t.Fatalf("Expected 1 build, got %d", len(builds))
}

if builds[0] != buildID {
    t.Errorf("Expected build ID %q, got %q", buildID, builds[0])
}
```

## How Build Discovery Works

The `discoverBuilds` function uses two methods to find active RSpecQ builds:

### Method 1: Status Key Scan (Primary)
```go
// Scan for keys matching pattern: *:status
statusIter := e.rdb.Scan(ctx, 0, "*:status", 1000).Iterator()
for statusIter.Next(ctx) {
    key := statusIter.Val()
    // Extract build ID from "<build_id>:status"
    parts := strings.Split(key, ":")
    if len(parts) >= 2 && parts[len(parts)-1] == "status" {
        buildID := strings.Join(parts[:len(parts)-1], ":")
        builds[buildID] = true
    }
}
```

**Examples of discovered builds:**
- Key: `build-123:status` → Build ID: `build-123`
- Key: `ci:master:run-1234:status` → Build ID: `ci:master:run-1234`
- Key: `project:branch:build:status` → Build ID: `project:branch:build`

### Method 2: Queue Key Scan (Fallback)
```go
// Scan for keys matching pattern: *:queue:*
queueIter := e.rdb.Scan(ctx, 0, "*:queue:*", 1000).Iterator()
for queueIter.Next(ctx) {
    key := queueIter.Val()
    parts := strings.Split(key, ":")
    if len(parts) >= 2 {
        buildID := parts[0]
        builds[buildID] = true
    }
}
```

**Examples of discovered builds:**
- Key: `build-123:queue:unprocessed` → Build ID: `build-123`
- Key: `build-456:queue:running` → Build ID: `build-456`
- Key: `build-789:queue:processed` → Build ID: `build-789`

## Complete Test Example

Here's a complete test that writes comprehensive data to Redis:

```go
func TestDiscoverBuilds_ViaStatusKey(t *testing.T) {
    // 1. Setup in-memory Redis
    rdb, _, cleanup := setupTestRedis(t)
    defer cleanup()

    ctx := context.Background()
    buildID := "build-via-status"

    // 2. Write status key to Redis
    err := rdb.Set(ctx, buildID+":status", "initializing", 0).Err()
    if err != nil {
        t.Fatalf("Failed to create status key: %v", err)
    }

    // 3. Create exporter and discover builds
    exporter := NewRSpecQExporter(rdb)
    builds, err := exporter.discoverBuilds(ctx)
    if err != nil {
        t.Fatalf("discoverBuilds failed: %v", err)
    }

    // 4. Verify results
    if len(builds) != 1 {
        t.Fatalf("Expected 1 build, got %d", len(builds))
    }

    if builds[0] != buildID {
        t.Errorf("Expected build ID %q, got %q", buildID, builds[0])
    }
}
```

## Testing Multiple Builds

```go
func TestDiscoverBuilds_MultipleBuilds(t *testing.T) {
    rdb, _, cleanup := setupTestRedis(t)
    defer cleanup()

    ctx := context.Background()
    buildIDs := []string{"build-1", "build-2", "build-3"}

    // Create status keys for multiple builds
    for _, buildID := range buildIDs {
        rdb.Set(ctx, buildID+":status", "ready", 0)
    }

    // Discover all builds
    exporter := NewRSpecQExporter(rdb)
    builds, err := exporter.discoverBuilds(ctx)

    // Should find all 3 builds
    if len(builds) != 3 {
        t.Fatalf("Expected 3 builds, got %d", len(builds))
    }
}
```

## Real-World Scenario Test

The `setupTestBuild` helper creates a complete RSpecQ build with all the data:

```go
func setupTestBuild(t *testing.T, ctx context.Context, rdb *redis.Client, buildID string) {
    // Queue data
    rdb.LPush(ctx, buildID+":queue:unprocessed", "job1", "job2")
    rdb.HSet(ctx, buildID+":queue:running", "worker-1", "job3")
    rdb.SAdd(ctx, buildID+":queue:processed", "job4", "job5", "job6")

    // Status
    rdb.Set(ctx, buildID+":status", "ready", 0)

    // Metrics
    rdb.Set(ctx, buildID+":example_count", "42", 0)
    rdb.HSet(ctx, buildID+":example_failures", "spec1", "failure1")

    // Worker heartbeats
    now := float64(time.Now().Unix())
    rdb.ZAdd(ctx, buildID+":worker_heartbeats",
        &redis.Z{Score: now, Member: "worker-1"},
    )
}
```

This allows testing the complete scraping cycle:

```go
func TestScrape_WithBuilds(t *testing.T) {
    rdb, _, cleanup := setupTestRedis(t)
    defer cleanup()

    ctx := context.Background()
    buildID := "test-build-scrape"

    // Create complete build data
    setupTestBuild(t, ctx, rdb, buildID)

    // Run scrape
    exporter := NewRSpecQExporter(rdb)
    exporter.scrape(ctx)

    // Verify metrics were collected
    queueUnprocessed := testutil.ToFloat64(
        exporter.buildQueueUnprocessed.WithLabelValues(buildID))
    if queueUnprocessed != 2.0 {
        t.Errorf("Expected 2 unprocessed jobs, got %f", queueUnprocessed)
    }
}
```

## Running the Tests

```bash
# Run all tests
go test -v

# Run only build discovery tests
go test -v -run TestDiscoverBuilds

# Run with coverage
go test -v -coverprofile=coverage.out

# View coverage report
go tool cover -html=coverage.out
```
