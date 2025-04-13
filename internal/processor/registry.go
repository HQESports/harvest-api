package processor

import (
	"sync"

	"github.com/rs/zerolog/log"
)

type BatchRegistry interface {
	Register(string, BaseBatchProcessor)
	Get(string) (BaseBatchProcessor, bool)
	AvailableProcessors() []string
}

// Registry is a central registry for job processors
type Registry struct {
	processors map[string]BaseBatchProcessor
	mu         sync.RWMutex
}

// NewRegistry creates a new processor registry
func NewRegistry(processors ...BaseBatchProcessor) BatchRegistry {
	registry := Registry{
		processors: make(map[string]BaseBatchProcessor),
	}

	for _, process := range processors {
		registry.Register(process.Type(), process)
	}

	return &registry
}

// Register adds a processor to the registry
func (r *Registry) Register(jobType string, processor BaseBatchProcessor) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.processors[jobType] = processor

	log.Info().
		Str("jobType", jobType).
		Str("processor", processor.Name()).
		Msg("Registered job processor")
}

// Get retrieves a processor by job type
func (r *Registry) Get(jobType string) (BaseBatchProcessor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processor, exists := r.processors[jobType]
	return processor, exists
}

// AvailableProcessors returns a list of all registered processor job types
func (r *Registry) AvailableProcessors() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processors := make([]string, 0, len(r.processors))
	for jobType := range r.processors {
		processors = append(processors, jobType)
	}

	return processors
}
