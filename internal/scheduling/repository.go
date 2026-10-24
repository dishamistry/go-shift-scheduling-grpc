package scheduling

import (
	"fmt"
	"sync"
)

// Repository stores shifts. It only persists data; business rules live in Service.
type Repository interface {
	Create(s Shift) error
	Get(id string) (Shift, error)
	Update(s Shift) error
	ListByEmployee(employeeID string) []Shift
}

// MemoryRepository is an in-memory Repository that is safe for concurrent use.
// Data is lost when the process stops.
type MemoryRepository struct {
	mu     sync.RWMutex
	shifts map[string]Shift
}

// NewMemoryRepository returns an empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{shifts: make(map[string]Shift)}
}

// Create stores a new shift. It fails if a shift with the same ID already exists.
func (r *MemoryRepository) Create(s Shift) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.shifts[s.ID]; exists {
		return fmt.Errorf("%w: shift %q already exists", ErrConflict, s.ID)
	}
	r.shifts[s.ID] = s
	return nil
}

// Get returns the shift with the given ID, or ErrNotFound.
func (r *MemoryRepository) Get(id string) (Shift, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.shifts[id]
	if !ok {
		return Shift{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	return s, nil
}

// Update replaces an existing shift, or returns ErrNotFound.
func (r *MemoryRepository) Update(s Shift) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.shifts[s.ID]; !ok {
		return fmt.Errorf("%w: %q", ErrNotFound, s.ID)
	}
	r.shifts[s.ID] = s
	return nil
}

// ListByEmployee returns all shifts assigned to the given employee, in no particular order.
func (r *MemoryRepository) ListByEmployee(employeeID string) []Shift {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Shift
	for _, s := range r.shifts {
		if s.EmployeeID == employeeID {
			result = append(result, s)
		}
	}
	return result
}
