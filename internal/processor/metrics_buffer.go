package processor

// Add to the top of the file, below imports
import (
	"context"
	"harvest/internal/model"
	"sync"
	"time"
)

// Add these new types and constants
const (
	// Default buffer flush interval in seconds
	DEFAULT_BUFFER_FLUSH_INTERVAL = 5
	// Default metrics buffer size
	DEFAULT_METRICS_BUFFER_SIZE = 10
)

// MetricsBuffer manages batched metric updates to reduce database calls
type MetricsBuffer struct {
	updates    []model.JobMetrics
	jobID      string
	processor  BaseBatchProcessor
	bufferSize int
	bufferChan chan model.JobMetrics
	ctx        context.Context
	mu         sync.Mutex
	done       chan struct{}
}

// NewMetricsBuffer creates a new metrics buffer with default settings
func NewMetricsBuffer(ctx context.Context, jobID string, processor BaseBatchProcessor) *MetricsBuffer {
	buffer := &MetricsBuffer{
		updates:    make([]model.JobMetrics, 0, DEFAULT_METRICS_BUFFER_SIZE),
		jobID:      jobID,
		processor:  processor,
		bufferSize: DEFAULT_METRICS_BUFFER_SIZE,
		bufferChan: make(chan model.JobMetrics, DEFAULT_METRICS_BUFFER_SIZE*2),
		ctx:        ctx,
		done:       make(chan struct{}),
	}

	// Start the buffer processor
	go buffer.processUpdates()

	return buffer
}

// Add queues a metrics update
func (b *MetricsBuffer) Add(metrics model.JobMetrics) {
	select {
	case b.bufferChan <- metrics:
		// Successfully queued
	case <-b.ctx.Done():
		// Context cancelled, discard update
	}
}

// Close flushes any pending updates and stops the processor
func (b *MetricsBuffer) Close() {
	close(b.done)
}

// processUpdates handles incoming metrics and periodic flushes
func (b *MetricsBuffer) processUpdates() {
	// Flush updates periodically
	ticker := time.NewTicker(time.Duration(DEFAULT_BUFFER_FLUSH_INTERVAL) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			// Final flush before exiting
			b.flush()
			return
		case <-b.done:
			// Buffer is being closed, flush and exit
			b.flush()
			return
		case metrics := <-b.bufferChan:
			b.mu.Lock()
			b.updates = append(b.updates, metrics)
			if len(b.updates) >= b.bufferSize {
				b.flushLocked()
			}
			b.mu.Unlock()
		case <-ticker.C:
			b.flush()
		}
	}
}

// flush acquires the lock and then flushes metrics
func (b *MetricsBuffer) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushLocked()
}

// flushLocked flushes metrics without acquiring the lock
func (b *MetricsBuffer) flushLocked() {
	if len(b.updates) == 0 {
		return
	}

	// Batch update metrics
	err := b.processor.UpdateMetrics(b.ctx, b.jobID, b.aggregateMetrics())
	if err != nil {
		// Log the error but continue processing
		// Could consider implementing retry logic here
	}

	// Clear the buffer
	b.updates = b.updates[:0]
}

// aggregateMetrics combines all buffered metrics into a single update
func (b *MetricsBuffer) aggregateMetrics() model.JobMetrics {
	total := model.JobMetrics{}
	for _, m := range b.updates {
		total.ProcessedItems += m.ProcessedItems
		total.SuccessCount += m.SuccessCount
		total.WarningCount += m.WarningCount
		total.FailureCount += m.FailureCount
		total.BatchesComplete += m.BatchesComplete
	}
	return total
}
