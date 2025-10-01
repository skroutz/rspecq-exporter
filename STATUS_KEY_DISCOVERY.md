# Build Discovery via `:status` Keys Only

## Overview

The RSpecQ exporter discovers builds **exclusively** by scanning for `<build-id>:status` keys in Redis. No other keys are used for discovery.

## Implementation

```go
// discoverBuilds finds all active builds by scanning Redis keys
// Builds are discovered by checking for <build_id>:status keys
func (e *RSpecQExporter) discoverBuilds(ctx context.Context) ([]string, error) {
    builds := make(map[string]bool)

    // Scan for status keys: <build_id>:status
    // This is the only method for discovering active builds
    statusIter := e.rdb.Scan(ctx, 0, "*:status", 1000).Iterator()
    for statusIter.Next(ctx) {
        key := statusIter.Val()
        // Extract build ID from "<build_id>:status"
        parts := strings.Split(key, ":")
        if len(parts) >= 2 && parts[len(parts)-1] == "status" {
            // Join all parts except the last one (which is "status")
            buildID := strings.Join(parts[:len(parts)-1], ":")
            builds[buildID] = true
        }
    }
    if err := statusIter.Err(); err != nil {
        return nil, err
    }

    buildList := make([]string, 0, len(builds))
    for buildID := range builds {
        buildList = append(buildList, buildID)
    }

    return buildList, nil
}
```

## Key Test: Only Status Keys Are Used

The most important test verifies that builds are **NOT** discovered via queue keys:

```go
func TestDiscoverBuilds_OnlyViaStatusKey(t *testing.T) {
    rdb, _, cleanup := setupTestRedis(t)
    defer cleanup()

    ctx := context.Background()

    // Create queue data WITHOUT a status key
    buildIDNoStatus := "build-without-status"
    rdb.LPush(ctx, buildIDNoStatus+":queue:unprocessed", "job1")
    rdb.SAdd(ctx, buildIDNoStatus+":queue:processed", "job2")

    // Create another build WITH status key
    buildIDWithStatus := "build-with-status"
    rdb.Set(ctx, buildIDWithStatus+":status", "ready", 0)

    exporter := NewRSpecQExporter(rdb)
    builds, err := exporter.discoverBuilds(ctx)
    if err != nil {
        t.Fatalf("discoverBuilds failed: %v", err)
    }

    // Should only find the build with status key
    if len(builds) != 1 {
        t.Fatalf("Expected 1 build (only via status key), got %d: %v", len(builds), builds)
    }

    if builds[0] != buildIDWithStatus {
        t.Errorf("Expected to find build %q, got %q", buildIDWithStatus, builds[0])
    }
}
```

**What this test does:**
1. Creates a build with queue data but **NO** `:status` key
2. Creates another build **WITH** a `:status` key
3. Verifies that **only the build with the status key** is discovered

## All Discovery Tests

### ✅ Basic Tests
- `TestDiscoverBuilds_NoBuilds` - No builds when no status keys exist
- `TestDiscoverBuilds_SingleBuild` - Discovers single build via status key
- `TestDiscoverBuilds_MultipleBuilds` - Discovers multiple builds via status keys

### ✅ Status Key Tests
- `TestDiscoverBuilds_ViaStatusKey` - Basic status key discovery
- `TestDiscoverBuilds_ViaStatusKeyMultipleBuilds` - Multiple builds via status keys
- `TestDiscoverBuilds_ViaStatusKeyWithComplexBuildID` - Complex build IDs with colons

### ✅ Critical Test
- `TestDiscoverBuilds_OnlyViaStatusKey` - **Verifies queue keys are ignored**

### ✅ Integration Tests
- `TestDiscoverBuilds_WithStatusKeys` - Status key with queue data present
- `TestDiscoverBuilds_DifferentQueueTypes` - All queue types with status key
- `TestDiscoverBuilds_IgnoresNonQueueKeys` - Non-build keys are ignored
- `TestDiscoverBuilds_CombinedDiscovery` - Multiple status keys with mixed data

## Pattern Matching

The discovery scans for `*:status` pattern and extracts build IDs:

| Redis Key | Discovered Build ID |
|-----------|-------------------|
| `build-123:status` | `build-123` |
| `ci:master:run-1234:status` | `ci:master:run-1234` |
| `project:branch:build:status` | `project:branch:build` |

## What's NOT Used for Discovery

❌ Queue keys: `<build-id>:queue:unprocessed`
❌ Queue keys: `<build-id>:queue:running`
❌ Queue keys: `<build-id>:queue:processed`
❌ Queue keys: `<build-id>:queue:lost`
❌ Any other keys without `:status` suffix

## Requirements

For a build to be discovered, it **MUST** have a `<build-id>:status` key in Redis.

## Test Results

```bash
$ go test -v -run TestDiscoverBuilds
=== RUN   TestDiscoverBuilds_NoBuilds
--- PASS: TestDiscoverBuilds_NoBuilds (0.00s)
=== RUN   TestDiscoverBuilds_SingleBuild
--- PASS: TestDiscoverBuilds_SingleBuild (0.00s)
=== RUN   TestDiscoverBuilds_MultipleBuilds
--- PASS: TestDiscoverBuilds_MultipleBuilds (0.00s)
=== RUN   TestDiscoverBuilds_WithStatusKeys
--- PASS: TestDiscoverBuilds_WithStatusKeys (0.00s)
=== RUN   TestDiscoverBuilds_DifferentQueueTypes
--- PASS: TestDiscoverBuilds_DifferentQueueTypes (0.00s)
=== RUN   TestDiscoverBuilds_IgnoresNonQueueKeys
--- PASS: TestDiscoverBuilds_IgnoresNonQueueKeys (0.00s)
=== RUN   TestDiscoverBuilds_ViaStatusKey
--- PASS: TestDiscoverBuilds_ViaStatusKey (0.00s)
=== RUN   TestDiscoverBuilds_ViaStatusKeyMultipleBuilds
--- PASS: TestDiscoverBuilds_ViaStatusKeyMultipleBuilds (0.00s)
=== RUN   TestDiscoverBuilds_ViaStatusKeyWithComplexBuildID
--- PASS: TestDiscoverBuilds_ViaStatusKeyWithComplexBuildID (0.00s)
=== RUN   TestDiscoverBuilds_CombinedDiscovery
--- PASS: TestDiscoverBuilds_CombinedDiscovery (0.00s)
=== RUN   TestDiscoverBuilds_OnlyViaStatusKey
--- PASS: TestDiscoverBuilds_OnlyViaStatusKey (0.00s)
PASS
```

All 11 discovery tests pass, confirming that build discovery works exclusively via `:status` keys.
