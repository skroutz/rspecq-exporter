package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "rspecq-exporter",
		Usage: "Prometheus exporter for RSpecQ metrics from Redis",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "redis-addr",
				Value:   "localhost:6379",
				Usage:   "Redis address",
				EnvVars: []string{"REDIS_ADDR", "REDIS_ADDRESS"},
			},
			&cli.StringFlag{
				Name:    "redis-password",
				Value:   "",
				Usage:   "Redis password",
				EnvVars: []string{"REDIS_PASSWORD"},
			},
			&cli.IntFlag{
				Name:    "redis-db",
				Value:   0,
				Usage:   "Redis database number",
				EnvVars: []string{"REDIS_DB", "REDIS_DATABASE"},
			},
			&cli.StringFlag{
				Name:    "listen-addr",
				Value:   ":9292",
				Usage:   "Address to listen on for metrics",
				EnvVars: []string{"LISTEN_ADDR", "LISTEN_ADDRESS"},
			},
			&cli.DurationFlag{
				Name:    "scrape-interval",
				Value:   15 * time.Second,
				Usage:   "Interval for scraping Redis metrics",
				EnvVars: []string{"SCRAPE_INTERVAL"},
			},
			&cli.BoolFlag{
				Name:    "disable-per-worker-metrics",
				Value:   false,
				Usage:   "Disable metrics about individual workers (reduces cardinality)",
				EnvVars: []string{"DISABLE_PER_WORKER_METRICS"},
			},
			&cli.StringFlag{
				Name:    "build-id-regex",
				Value:   "",
				Usage:   "Named capture group regex to extract labels from build IDs (e.g., '(?P<project>\\w+)-(?P<branch>\\w+)-(?P<build>\\d+)')",
				EnvVars: []string{"BUILD_ID_REGEX"},
			},
		},
		Action: func(c *cli.Context) error {
			return run(c)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(c *cli.Context) error {

	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     c.String("redis-addr"),
		Password: c.String("redis-password"),
		DB:       c.Int("redis-db"),
	})

	ctx := context.Background()

	// Test Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Successfully connected to Redis at %s", c.String("redis-addr"))

	// Create and register the exporter
	exporter, err := NewRSpecQExporter(rdb, c.Bool("disable-per-worker-metrics"), c.String("build-id-regex"))
	if err != nil {
		log.Fatalf("Failed to create exporter: %v", err)
	}
	prometheus.MustRegister(exporter)

	// Setup HTTP server
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>RSpecQ Exporter</title></head>
			<body>
			<h1>RSpecQ Exporter</h1>
			<p><a href="/metrics">Metrics</a></p>
			</body>
			</html>`))
	})

	// Start background scraper
	go exporter.StartScraper(ctx, c.Duration("scrape-interval"))

	// Setup graceful shutdown
	srv := &http.Server{Addr: c.String("listen-addr")}

	go func() {
		log.Printf("Starting HTTP server on %s", c.String("listen-addr"))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	if err := rdb.Close(); err != nil {
		log.Printf("Redis close error: %v", err)
	}

	log.Println("Exporter stopped")
	return nil
}
