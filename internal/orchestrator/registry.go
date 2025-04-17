package orchestrator

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
)

type WorkerRegistry interface {
	Register(string, BatchWorker)
	Get(string) (BatchWorker, bool)
	AvailableProcessors() []string
	CancelProcessByType(string) error
}

// Registry is a central registry for job processors
type Registry struct {
	processors map[string]BatchWorker
	mu         sync.RWMutex
}

// NewRegistry creates a new processor registry
func NewWorkerRegistry(processors ...BatchWorker) WorkerRegistry {
	registry := Registry{
		processors: make(map[string]BatchWorker),
	}

	for _, process := range processors {
		registry.Register(process.Type(), process)
	}

	return &registry
}

func (r *Registry) CancelProcessByType(processType string) error {
	process, ok := r.Get(processType)

	if !ok {
		return fmt.Errorf("process not found, can't cancel")
	}

	return process.Cancel()
}

// Register adds a processor to the registry
func (r *Registry) Register(jobType string, processor BatchWorker) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.processors[jobType] = processor

	log.Info().
		Str("jobType", jobType).
		Str("processor", processor.Name()).
		Msg("Registered job processor")
}

// Get retrieves a processor by job type
func (r *Registry) Get(jobType string) (BatchWorker, bool) {
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
