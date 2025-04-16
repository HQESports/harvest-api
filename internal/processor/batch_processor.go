package processor

import (
	"context"
	"harvest/internal/model"
	"sync"
	"sync/atomic"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BaseBatchProcessor interface {
	// ProcessBatch processes a job and returns results
	StartJob(*model.Job) ([]model.JobResult, error)

	// Cancels the job
	Cancel() error

	// UpdateMetrics updates job metrics during processing
	// Note: Changed jobID parameter type from string to primitive.ObjectID.Hex()
	UpdateMetrics(ctx context.Context, jobID string, metrics model.JobMetrics) error

	// Name returns the processor name
	Name() string

	// Is Active
	IsActive() bool

	// Type returns the type of the processor as a string
	Type() string
}

type StringBatchProcessor interface {
	BaseBatchProcessor
	Operation(context.Context, string, primitive.ObjectID) StatusError
}

type StringSliceBatchProcessor interface {
	BaseBatchProcessor
	Operation(context.Context, []string, primitive.ObjectID) StatusError
}

// Modified ProcessBatch to reduce database calls
func ProcessBatch[T any](
	ctx context.Context,
	items []T,
	operation func(context.Context, T, primitive.ObjectID) StatusError,
	batchSize int,
	processor BaseBatchProcessor,
	jobID primitive.ObjectID,
) model.JobMetrics {
	// Initialize metrics to track the overall progress
	totalMetrics := model.JobMetrics{
		ProcessedItems:  0,
		SuccessCount:    0,
		WarningCount:    0,
		FailureCount:    0,
		BatchesComplete: 0,
	}

	// Create metrics buffer to reduce DB calls
	buffer := NewMetricsBuffer(ctx, jobID.Hex(), processor)
	defer buffer.Close()

	// Process each batch sequentially
	batches := SplitIntoBatches(items, batchSize)
	for i, batch := range batches {
		// Check for cancellation
		if ctx.Err() != nil {
			return totalMetrics
		}

		// Process the batch
		batchMetrics := processSingleBatch(ctx, batch, operation, jobID)

		// Add to buffer (will be flushed automatically)
		buffer.Add(batchMetrics)

		// Update our total metrics for tracking
		totalMetrics.ProcessedItems += batchMetrics.ProcessedItems
		totalMetrics.SuccessCount += batchMetrics.SuccessCount
		totalMetrics.WarningCount += batchMetrics.WarningCount
		totalMetrics.FailureCount += batchMetrics.FailureCount
		totalMetrics.BatchesComplete++

		// For long-running processes, provide periodic complete snapshots
		// but only every 5 batches to reduce DB calls
		if i > 0 && i%5 == 0 {
			// Force a manual update with current totals (not incremental)
			// This ensures the client has an accurate view periodically
			snapshotMetrics := totalMetrics
			processor.UpdateMetrics(ctx, jobID.Hex(), snapshotMetrics)
		}
	}

	return totalMetrics
}

// Helper function to process a single batch
func processSingleBatch[T any](
	ctx context.Context,
	batch []T,
	operation func(context.Context, T, primitive.ObjectID) StatusError,
	jobID primitive.ObjectID,
) model.JobMetrics {
	batchMetrics := model.JobMetrics{BatchesComplete: 1}

	var wg sync.WaitGroup
	wg.Add(len(batch))

	// Use atomic operations for thread safety
	var processedItems, successCount, warningCount, failureCount int32

	// Process each item concurrently
	for i, item := range batch {
		_, item := i, item
		go func() {
			defer wg.Done()

			if ctx.Err() != nil {
				atomic.AddInt32(&warningCount, 1)
				atomic.AddInt32(&processedItems, 1)
				return
			}

			result := operation(ctx, item, jobID)

			// Use atomic operations for thread safety
			switch result.Status() {
			case StatusSuccess:
				atomic.AddInt32(&successCount, 1)
			case StatusWarning:
				atomic.AddInt32(&warningCount, 1)
			case StatusFailure:
				atomic.AddInt32(&failureCount, 1)
			}
			atomic.AddInt32(&processedItems, 1)
		}()
	}

	wg.Wait()

	// Convert atomic counters to metrics
	batchMetrics.ProcessedItems = int(processedItems)
	batchMetrics.SuccessCount = int(successCount)
	batchMetrics.WarningCount = int(warningCount)
	batchMetrics.FailureCount = int(failureCount)

	return batchMetrics
}

// SplitIntoBatches is a generic function that divides a slice of items
// into batches of the specified size
func SplitIntoBatches[T any](items []T, batchSize int) [][]T {
	// Handle edge cases
	if batchSize <= 0 {
		return nil
	}

	if len(items) == 0 {
		return [][]T{}
	}

	// Calculate the number of batches needed
	numBatches := (len(items) + batchSize - 1) / batchSize

	// Create the result slice with the right capacity
	batches := make([][]T, 0, numBatches)

	// Split the items into batches
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize

		// Handle the last batch which might be smaller
		if end > len(items) {
			end = len(items)
		}

		// Create a new batch with the current slice of items
		batches = append(batches, items[i:end])
	}

	return batches
}
