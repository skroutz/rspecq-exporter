package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	redisAddr               = flag.String("redis-addr", "localhost:6379", "Redis address")
	redisPassword           = flag.String("redis-password", "", "Redis password")
	redisDB                 = flag.Int("redis-db", 0, "Redis database number")
	listenAddr              = flag.String("listen-addr", ":9292", "Address to listen on for metrics")
	scrapeInterval          = flag.Duration("scrape-interval", 15*time.Second, "Interval for scraping Redis metrics")
	disablePerWorkerMetrics = flag.Bool("disable-per-worker-metrics", false, "Disable metrics about individual workers (reduces cardinality)")
)

func main() {
	flag.Parse()

	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     *redisAddr,
		Password: *redisPassword,
		DB:       *redisDB,
	})

	ctx := context.Background()

	// Test Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Successfully connected to Redis at %s", *redisAddr)

	// Create and register the exporter
	exporter := NewRSpecQExporter(rdb, *disablePerWorkerMetrics)
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
	go exporter.StartScraper(ctx, *scrapeInterval)

	// Setup graceful shutdown
	srv := &http.Server{Addr: *listenAddr}

	go func() {
		log.Printf("Starting HTTP server on %s", *listenAddr)
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
}
