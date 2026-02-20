package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/singleflight"
)

type FileHandler struct {
	baseDir     string
	cache       *MemoryCache
	sfGroup     singleflight.Group
	checkTime   time.Duration
	minSpeed    float64
	hedgedDelay time.Duration
}

func NewFileHandler(baseDir string, cache *MemoryCache, checkTime time.Duration, minSpeed float64, hedgedDelay time.Duration) *FileHandler {
	return &FileHandler{
		baseDir:     baseDir,
		cache:       cache,
		checkTime:   checkTime,
		minSpeed:    minSpeed,
		hedgedDelay: hedgedDelay,
	}
}

func (h *FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clean path and prevent directory traversal
	cleanPath := filepath.Clean(r.URL.Path)
	if cleanPath == "/" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	filePath := filepath.Join(h.baseDir, cleanPath)

	// Check cache first
	if data, ok := h.cache.Get(filePath); ok {
		log.Printf("Cache hit for %s", cleanPath)
		h.serveBytes(w, r, filePath, data)
		return
	}

	// Use singleflight to prevent cache stampedes
	val, err, _ := h.sfGroup.Do(filePath, func() (interface{}, error) {
		// Singleflight execution
		return h.readHedged(r.Context(), filePath)
	})

	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			log.Printf("Error reading file %s: %v", cleanPath, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	data := val.([]byte)

	// Cache the result
	h.cache.Set(filePath, data)

	// Serve the buffer
	h.serveBytes(w, r, filePath, data)
}

func (h *FileHandler) serveBytes(w http.ResponseWriter, r *http.Request, filePath string, data []byte) {
	// We could use http.ServeContent to support Range requests properly
	// By wrapping our byte slice in a bytes.Reader
	seeker := bytes.NewReader(data)
	
	// We don't have the original file modtime easily without an extra stat,
	// but ServeContent will handle the range logic at least.
	http.ServeContent(w, r, filepath.Base(filePath), time.Time{}, seeker)
}

// readHedged implements the hedging read logic:
// First try -> Slow Abort (if speed < minSpeed within checkTime) -> Delay -> Second try
func (h *FileHandler) readHedged(ctx context.Context, filePath string) ([]byte, error) {
	log.Printf("First try reading %s", filepath.Base(filePath))
	data, err := h.doRead(ctx, filePath, true)
	if err == nil {
		return data, nil
	}

	if errors.Is(err, ErrTooSlow) {
		log.Printf("First try for %s too slow, aborting and hedging...", filepath.Base(filePath))
		// Pause briefly to let the kernel pull data into Page Cache
		time.Sleep(h.hedgedDelay)

		log.Printf("Second try (hedged) for %s", filepath.Base(filePath))
		// Second try without the speed limit abort, or we could apply it again.
		// According to the design, second try should just attempt to read (hopefully hitting page cache).
		return h.doRead(ctx, filePath, false)
	}

	return nil, err
}

func (h *FileHandler) doRead(ctx context.Context, filePath string, useSpeedLimit bool) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var reader io.Reader = file
	if useSpeedLimit {
		reader = NewHedgingReader(ctx, file, h.checkTime, h.minSpeed)
	}

	var buf bytes.Buffer
	chunk := make([]byte, 1024*1024) // 1MB chunks

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		n, readErr := reader.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, readErr
		}
	}

	return buf.Bytes(), nil
}
