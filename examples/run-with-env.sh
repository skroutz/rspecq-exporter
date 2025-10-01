#!/bin/bash
# Example script demonstrating how to run rspecq-exporter with environment variables

# Redis connection settings
export REDIS_ADDR="localhost:6379"
export REDIS_PASSWORD=""  # Leave empty if no password
export REDIS_DB="0"

# Server settings
export LISTEN_ADDR=":9292"
export SCRAPE_INTERVAL="15s"

# Optional: Extract custom labels from build IDs
# Example pattern for build IDs like "my-project-main-12345"
export BUILD_ID_REGEX='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'

# Optional: Reduce cardinality by disabling per-worker metrics
export DISABLE_PER_WORKER_METRICS="false"

# Start the exporter
# All configuration is read from environment variables
./rspecq-exporter
