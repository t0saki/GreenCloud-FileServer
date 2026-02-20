package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	// Setup command line arguments for configuration
	dirPtr := flag.String("dir", "./data", "Directory to serve files from")
	portPtr := flag.Int("port", 8080, "Port to listen on")
	maxBytesPtr := flag.Int64("cacheSizeBytes", 1024*1024*1024, "Maximum memory cache size in bytes (default 1GB)")
	checkTimePtr := flag.Duration("checkTime", 1*time.Second, "Time to check speed after")
	minSpeedPtr := flag.Float64("minSpeedMbps", 5.0, "Minimum speed in Mbps before aborting")
	hedgedDelayPtr := flag.Duration("hedgedDelay", 100*time.Millisecond, "Time to wait before second read attempt")

	flag.Parse()

	// Environment variable overrides
	if envDir := os.Getenv("SERVE_DIR"); envDir != "" {
		*dirPtr = envDir
	}
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*portPtr = p
		}
	}
	if envCacheSize := os.Getenv("CACHE_SIZE_BYTES"); envCacheSize != "" {
		if c, err := strconv.ParseInt(envCacheSize, 10, 64); err == nil {
			*maxBytesPtr = c
		}
	}

	// Ensure the base directory exists
	if _, err := os.Stat(*dirPtr); os.IsNotExist(err) {
		log.Printf("Warning: Serving directory %s does not exist, creating it.", *dirPtr)
		os.MkdirAll(*dirPtr, 0755)
	}

	// Initialize the memory cache
	log.Printf("Initializing memory cache (Max Size: %d bytes)", *maxBytesPtr)
	cache := NewMemoryCache(*maxBytesPtr)

	// Initialize the file handler
	log.Printf("Initializing file handler (Hedged threshold: %.2f Mbps after %v)", *minSpeedPtr, *checkTimePtr)
	handler := NewFileHandler(*dirPtr, cache, *checkTimePtr, *minSpeedPtr, *hedgedDelayPtr)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.Handle("/", handler)

	addr := ":" + strconv.Itoa(*portPtr)
	log.Printf("Server listening on %s", addr)
	
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
