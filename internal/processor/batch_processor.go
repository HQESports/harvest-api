package processor

import (
	"context"
	"harvest/internal/model"
	"runtime"
	"sync"
)

type BaseBatchProcessor interface {
	// ProcessBatch processes a job and returns results
	ProcessBatch(context.Context, *model.Job) ([]model.JobResult, error)

	UpdateMetrics(ctx context.Context, jobID string, metrics model.JobMetrics)

	// Name returns the processor name
	Name() string

	// Type returns the type of the processor as a string
	Type() string
}

type StringBatchProcessor interface {
	BaseBatchProcessor
	Operation(string) StatusError
}

type StringSliceBatchProcessor interface {
	BaseBatchProcessor
	Operation([]string) StatusError
}

// ProcessBatch is a generic function that processes a batch of items concurrently
// and tracks status counts
func ProcessBatch[T any](items []T, operation func(T) StatusError, maxConcurrency int) model.JobMetrics {
	// If maxConcurrency is not specified or invalid, set a reasonable default
	if maxConcurrency <= 0 {
		// Default to number of CPU cores for a reasonable balance
		maxConcurrency = runtime.NumCPU()
	}

	// Create metrics for tracking results
	operationMetrics := model.JobMetrics{}

	// Create a mutex to protect the counters
	var mutex sync.Mutex

	// Create a wait group to synchronize all work
	var wg sync.WaitGroup

	// Create a channel to feed work to the worker pool
	// Buffer it to improve throughput
	workChan := make(chan struct {
		index int
		item  T
	}, min(len(items), 1000)) // Limit buffer size to avoid excessive memory usage

	// Start the worker pool with the specified concurrency
	wg.Add(maxConcurrency)
	for i := 0; i < maxConcurrency; i++ {
		go func() {
			defer wg.Done()

			// Process items from the work channel until it's closed
			for work := range workChan {
				// Apply the operation to the item
				status := operation(work.item)

				// Update metrics with proper synchronization
				mutex.Lock()
				switch status.Status {
				case Success:
					operationMetrics.SuccessCount++
				case Warning:
					operationMetrics.WarningCount++
				case Failure:
					operationMetrics.FailureCount++
				}
				mutex.Unlock()
			}
		}()
	}

	// Feed all items to the work channel
	for i, item := range items {
		workChan <- struct {
			index int
			item  T
		}{i, item}
	}
	close(workChan) // Signal workers that no more work is coming

	// Wait for all workers to complete
	wg.Wait()

	return operationMetrics
}

// Helper function to return the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// Type-specific wrapper functions for backward compatibility
func ProcessStringsBatch(strings []string, operation func(string) StatusError, batchSize int) model.JobMetrics {
	return ProcessBatch(strings, operation, batchSize)
}

func ProcessStringSlicesBatch(stringSlices [][]string, operation func([]string) StatusError, batchSize int) model.JobMetrics {
	return ProcessBatch(stringSlices, operation, batchSize)
}
