package ofr

import (
	"sync"

	"github.com/HdrHistogram/hdrhistogram-go"
)

const (
	p50 = 50.0
	p75 = 75.0
	p90 = 90.0
	p99 = 99.0
)

// Histogram tracks int64 values with percentiles
// Uses a buffered channel to record values in a lock-free manner
type Histogram struct {
	histogram *hdrhistogram.Histogram
	recordCh  chan int64
	mu        sync.Mutex
}

// HistogramResult contains percentile statistics
type HistogramResult struct {
	P50           int64
	P75           int64
	P90           int64
	P99           int64
	Max           int64
	TotalRecorded uint64
}

// NewHistogram creates a new histogram for 1 to 10,000,000 range with 3 significant figures
// This gives accurate percentiles with minimal memory (~30KB)
func NewHistogram() *Histogram {
	h := &Histogram{
		recordCh:  make(chan int64, 1e4),
		mu:        sync.Mutex{},
		histogram: hdrhistogram.New(1, 10000000, 3),
	}

	go h.recordingRoutine()

	return h
}

func (h *Histogram) recordingRoutine() {
	for value := range h.recordCh {
		h.mu.Lock()
		h.histogram.RecordValue(value)
		h.mu.Unlock()
	}
}

// Record records a value
// Non-blocking send to channel (lock-free on hot path)
func (h *Histogram) Record(value int64) {
	select {
	case h.recordCh <- value:
	default:
	}
}

// GetStats computes percentiles from histogram
func (h *Histogram) GetStats() HistogramResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	totalRecorded := uint64(h.histogram.TotalCount())

	if totalRecorded == 0 {
		return HistogramResult{
			TotalRecorded: totalRecorded,
		}
	}

	valuesAtPercentiles := []float64{p50, p75, p90, p99}
	results := h.histogram.ValueAtPercentiles(valuesAtPercentiles)

	return HistogramResult{
		P50:           results[p50],
		P75:           results[p75],
		P90:           results[p90],
		P99:           results[p99],
		Max:           h.histogram.Max(),
		TotalRecorded: totalRecorded,
	}
}

// Reset clears the histogram for the next window
func (h *Histogram) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.histogram.Reset()
}
