# Build ID Regex Label Extraction

This document explains how to use the `--build-id-regex` flag to extract custom labels from build IDs.

## Overview

The `--build-id-regex` flag allows you to parse build IDs using named capture groups in a regular expression. Any named groups in the regex will be extracted and added as labels to all build-level metrics.

## Usage

```bash
./rspecq-exporter --build-id-regex='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'
```

## Examples

### Example 1: Extract Project, Branch, and Build Number

If your build IDs follow the pattern `my-project-main-12345`, you can extract labels like this:

**Regex:**
```regex
(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)
```

**Build ID:** `my-project-main-12345`

**Extracted Labels:**
- `build_id`: `my-project-main-12345` (always included)
- `project`: `my-project`
- `branch`: `main`
- `build`: `12345`

**Metrics Example:**
```
rspecq_build_queue_unprocessed{build_id="my-project-main-12345",project="my-project",branch="main",build="12345"} 3
rspecq_build_example_count{build_id="my-project-main-12345",project="my-project",branch="main",build="12345"} 150
```

### Example 2: Extract Environment and Service

If your build IDs follow the pattern `production-api-v1.2.3`:

**Regex:**
```regex
(?P<env>[^-]+)-(?P<service>[^-]+)-(?P<version>.+)
```

**Build ID:** `production-api-v1.2.3`

**Extracted Labels:**
- `build_id`: `production-api-v1.2.3`
- `env`: `production`
- `service`: `api`
- `version`: `v1.2.3`

### Example 3: Extract CI/CD Information

If your build IDs include CI/CD information like `ci-jenkins-feature_auth-build_456`:

**Regex:**
```regex
(?P<ci_system>\w+)-(?P<platform>\w+)-(?P<feature>[\w_]+)-build_(?P<number>\d+)
```

**Build ID:** `ci-jenkins-feature_auth-build_456`

**Extracted Labels:**
- `build_id`: `ci-jenkins-feature_auth-build_456`
- `ci_system`: `ci`
- `platform`: `jenkins`
- `feature`: `feature_auth`
- `number`: `456`

## Prometheus Queries

With these labels, you can create more powerful Prometheus queries:

### Query builds by project
```promql
rspecq_build_queue_unprocessed{project="my-project"}
```

### Query builds by branch
```promql
rspecq_build_example_failures{branch="main"}
```

### Aggregate by environment
```promql
sum(rspecq_build_example_count) by (env)
```

### Filter by specific build range
```promql
rspecq_build_duration_seconds{project="my-project",build=~"123.*"}
```

## Important Notes

1. **Always include `build_id`**: The `build_id` label is always present, even without a regex
2. **Regex must be valid**: Invalid regex patterns will cause the exporter to fail at startup
3. **Named groups only**: Only named capture groups (e.g., `(?P<name>...)`) will be extracted as labels
4. **Non-matching IDs**: If a build ID doesn't match the regex, only the `build_id` label will be present
5. **Cardinality**: Be mindful of label cardinality - more labels means more time series in Prometheus

## Testing Your Regex

Before deploying, you can test your regex pattern:

```bash
# Test with dry-run (if supported) or check logs on startup
./rspecq-exporter --build-id-regex='your-pattern-here' --help
```

The exporter will validate the regex pattern on startup and fail fast if it's invalid.

## Regex Syntax Reference

Go uses the RE2 regex syntax. Some common patterns:

- `\w` - Word characters (letters, digits, underscore)
- `\d` - Digits
- `[^-]` - Any character except hyphen
- `[\w-]+` - One or more word characters or hyphens
- `.+` - One or more of any character
- `(?P<name>...)` - Named capture group

For complete syntax, see: https://github.com/google/re2/wiki/Syntax
