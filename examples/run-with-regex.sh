#!/bin/bash
# Example: Running rspecq-exporter with build ID regex extraction

# Example 1: Extract project, branch, and build number from build IDs like "myapp-main-12345"
./rspecq-exporter \
  --redis-addr=localhost:6379 \
  --build-id-regex='(?P<project>[\w-]+)-(?P<branch>\w+)-(?P<build>\d+)'

# Example 2: Extract environment and service from build IDs like "production-api-v1.2.3"
# ./rspecq-exporter \
#   --redis-addr=localhost:6379 \
#   --build-id-regex='(?P<env>[^-]+)-(?P<service>[^-]+)-(?P<version>.+)'

# Example 3: Complex pattern with CI information like "ci-jenkins-project-branch-123"
# ./rspecq-exporter \
#   --redis-addr=localhost:6379 \
#   --build-id-regex='(?P<ci_system>\w+)-(?P<platform>\w+)-(?P<project>\w+)-(?P<branch>\w+)-(?P<build>\d+)'

# Example 4: No regex - just use build_id as is
# ./rspecq-exporter \
#   --redis-addr=localhost:6379
