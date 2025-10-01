#!/bin/bash
# Initialize and test the RSpecQ Exporter project
# This script helps you quickly validate the setup

set -e

echo "================================================"
echo "RSpecQ Exporter - Setup & Validation Script"
echo "================================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check Go installation
echo "Checking prerequisites..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    print_status "Go is installed: $GO_VERSION"
else
    print_error "Go is not installed. Please install Go 1.21 or later."
    exit 1
fi

# Check Docker (optional)
if command -v docker &> /dev/null; then
    print_status "Docker is installed"
    DOCKER_AVAILABLE=true
else
    print_warning "Docker is not installed (optional for containerization)"
    DOCKER_AVAILABLE=false
fi

echo ""
echo "Installing dependencies..."
go mod download
go mod tidy
print_status "Dependencies installed"

echo ""
echo "Running tests..."
if go test -short ./...; then
    print_status "Tests passed"
else
    print_warning "Some tests failed (may need Redis for integration tests)"
fi

echo ""
echo "Building the project..."
if go build -o rspecq-exporter .; then
    print_status "Build successful: ./rspecq-exporter"
else
    print_error "Build failed"
    exit 1
fi

echo ""
echo "Project is ready! Next steps:"
echo ""
echo "1. Start Redis (if not running):"
echo "   ${YELLOW}docker run -d -p 6379:6379 redis:7-alpine${NC}"
echo ""
echo "2. Run the exporter:"
echo "   ${YELLOW}./rspecq-exporter --redis-addr=localhost:6379${NC}"
echo ""
echo "3. View metrics:"
echo "   ${YELLOW}curl http://localhost:9292/metrics${NC}"
echo ""
echo "4. For full stack with Prometheus and Grafana:"
echo "   ${YELLOW}docker-compose up -d${NC}"
echo ""
echo "Documentation:"
echo "  - Quick start: ${YELLOW}QUICKSTART.md${NC}"
echo "  - Main docs:   ${YELLOW}README.md${NC}"
echo "  - Roadmap:     ${YELLOW}ACTION_PLAN.md${NC}"
echo ""
print_status "Setup complete!"
