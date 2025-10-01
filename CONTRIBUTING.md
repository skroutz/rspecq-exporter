# Contributing to RSpecQ Exporter

Thank you for your interest in contributing to the RSpecQ Exporter! This document provides guidelines and information for contributors.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/yourusername/rspecq-exporter.git`
3. Create a feature branch: `git checkout -b feature/my-feature`
4. Make your changes
5. Run tests: `make test`
6. Commit your changes: `git commit -am 'Add some feature'`
7. Push to the branch: `git push origin feature/my-feature`
8. Create a Pull Request

## Development Setup

### Prerequisites

- Go 1.21 or later
- Redis 7.x (for integration tests)
- Docker (optional, for containerized testing)

### Local Development

```bash
# Clone the repository
git clone https://github.com/yourusername/rspecq-exporter.git
cd rspecq-exporter

# Install dependencies
go mod download

# Run tests
make test

# Build the binary
make build

# Run locally
./rspecq-exporter --redis-addr=localhost:6379
```

### Running Tests

```bash
# Unit tests only
go test -short ./...

# All tests (requires Redis)
docker run -d -p 6379:6379 redis:7-alpine
make test

# With coverage
make coverage
```

## Code Style

- Follow standard Go conventions
- Run `go fmt` before committing
- Run `go vet` to catch common mistakes
- Use `golangci-lint` for comprehensive linting

```bash
make fmt
make vet
make lint
```

## Commit Messages

- Use clear and descriptive commit messages
- Start with a verb in present tense (e.g., "Add", "Fix", "Update")
- Reference issue numbers when applicable
- Keep the first line under 72 characters

Examples:
```
Add worker health metrics based on heartbeat staleness

Fix Redis connection handling for timeouts

Update documentation for metric labels
```

## Pull Request Process

1. Update the README.md with details of changes if needed
2. Update the ACTION_PLAN.md if adding new features
3. Add tests for new functionality
4. Ensure all tests pass
5. Update documentation
6. Request review from maintainers

## Testing Guidelines

### Unit Tests

- Test individual functions in isolation
- Mock external dependencies (Redis, etc.)
- Use table-driven tests when appropriate
- Aim for >80% code coverage

### Integration Tests

- Test end-to-end functionality
- Use real Redis instance (or testcontainers)
- Clean up test data after each test
- Tag integration tests with `//go:build integration`

### Example Test Structure

```go
func TestCollectBuildMetrics(t *testing.T) {
    tests := []struct {
        name     string
        buildID  string
        expected float64
        wantErr  bool
    }{
        {"active build", "build-123", 10, false},
        {"empty build", "build-empty", 0, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## Adding New Metrics

When adding new metrics:

1. Define the metric in `exporter.go`
2. Initialize it in `NewRSpecQExporter()`
3. Add it to `Describe()` and `Collect()` methods
4. Implement collection logic
5. Add tests
6. Document in README.md

## Documentation

- Add Go doc comments for all exported functions/types
- Update README.md for user-facing changes
- Add examples for new features
- Keep ACTION_PLAN.md updated

## Issue Reporting

When reporting issues, include:

- RSpecQ version
- Redis version
- Go version
- Exporter version
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs/metrics

## Feature Requests

Feature requests are welcome! Please:

1. Check if the feature already exists
2. Search for existing feature requests
3. Provide clear use case and benefits
4. Be open to discussion and alternatives

## Code Review

All submissions require review. We use GitHub pull requests for this purpose.

Reviewers will check for:

- Code quality and style
- Test coverage
- Documentation
- Performance implications
- Backward compatibility

## Community

- Be respectful and constructive
- Help others when possible
- Follow the Code of Conduct
- Share knowledge and learn together

## Questions?

If you have questions, feel free to:

- Open an issue for discussion
- Comment on existing issues/PRs
- Reach out to maintainers

Thank you for contributing!
