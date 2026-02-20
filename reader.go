package main

import (
	"context"
	"errors"
	"os"
	"time"
)

var ErrTooSlow = errors.New("I/O Too Slow, Abort and Retry")

// HedgingReader wraps a file to monitor its read speed.
type HedgingReader struct {
	file      *os.File
	ctx       context.Context
	startTime time.Time
	bytesRead int64
	checkTime time.Duration
	minSpeed  float64 // Mbps
}

func NewHedgingReader(ctx context.Context, file *os.File, checkTime time.Duration, minSpeed float64) *HedgingReader {
	return &HedgingReader{
		file:      file,
		ctx:       ctx,
		startTime: time.Now(),
		checkTime: checkTime,
		minSpeed:  minSpeed,
	}
}

func (r *HedgingReader) Read(p []byte) (n int, err error) {
	// Respect context cancellation
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	n, err = r.file.Read(p)
	if n > 0 {
		r.bytesRead += int64(n)
	}

	// Check speed if we are past the checkTime interval
	elapsed := time.Since(r.startTime)
	if elapsed >= r.checkTime {
		// Speed in Mbps: (bytes * 8) / (1024 * 1024) / seconds
		speedMbps := (float64(r.bytesRead) * 8) / (1024 * 1024 * elapsed.Seconds())
		if speedMbps < r.minSpeed {
			// If it's too slow, check if we're near the end of the file.
			// Sometimes very small files or the last few kb can skew the speed.
			// But for simplicity, we just abort.
			return n, ErrTooSlow
		}
	}

	return n, err
}
