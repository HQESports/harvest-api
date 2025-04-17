package orchestrator

import (
	"harvest/internal/model"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BatchWorker interface {
	// ProcessBatch processes a job and returns results
	StartWorker(*model.Job) (bool, error)

	// Cancels the job
	Cancel() error

	// Name returns the processor name
	Name() string

	// Is Active
	IsActive() bool

	// Type returns the type of the processor as a string
	Type() string

	//
	ActiveJobID() *primitive.ObjectID
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
