# Build stage
FROM golang:1.21-bookworm AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -o rspecq-exporter .

# Final stage
FROM debian:bookworm-slim

# Install ca-certificates for HTTPS connections
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/rspecq-exporter .

# Create a non-root user
RUN useradd -r -u 1000 -s /bin/false exporter && \
    chown -R exporter:exporter /app

USER exporter

EXPOSE 9292

ENTRYPOINT ["./rspecq-exporter"]
