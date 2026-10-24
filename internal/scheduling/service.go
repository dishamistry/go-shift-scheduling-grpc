package scheduling

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Service implements the shift scheduling business rules on top of a Repository.
type Service struct {
	repo  Repository
	newID func() string

	// assignMu serializes assignments so the overlap check and the update
	// happen as one step. Without it, two concurrent requests could both pass
	// the check and assign the same employee to overlapping shifts.
	assignMu sync.Mutex
}

// NewService returns a Service that stores shifts in repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, newID: rand.Text}
}

// CreateShift creates a new, unassigned shift. Times are stored in UTC.
func (s *Service) CreateShift(start, end time.Time) (Shift, error) {
	if err := validateTimeRange(start, end); err != nil {
		return Shift{}, err
	}

	shift := Shift{
		ID:    s.newID(),
		Start: start.UTC(),
		End:   end.UTC(),
	}
	if err := s.repo.Create(shift); err != nil {
		return Shift{}, err
	}
	return shift, nil
}

// GetShift returns the shift with the given ID.
func (s *Service) GetShift(id string) (Shift, error) {
	id, err := normalizeID("shift id", id)
	if err != nil {
		return Shift{}, err
	}
	return s.repo.Get(id)
}

// AssignEmployee assigns an employee to an unassigned shift.
//
// It fails with ErrConflict if the shift is already assigned to someone else,
// or if the employee already has a shift that overlaps this one. Assigning
// the same employee again is allowed and returns the shift unchanged.
func (s *Service) AssignEmployee(shiftID, employeeID string) (Shift, error) {
	shiftID, err := normalizeID("shift id", shiftID)
	if err != nil {
		return Shift{}, err
	}
	employeeID, err = normalizeID("employee id", employeeID)
	if err != nil {
		return Shift{}, err
	}

	s.assignMu.Lock()
	defer s.assignMu.Unlock()

	shift, err := s.repo.Get(shiftID)
	if err != nil {
		return Shift{}, err
	}
	if shift.EmployeeID == employeeID {
		return shift, nil
	}
	if shift.IsAssigned() {
		return Shift{}, fmt.Errorf("%w: shift %q is already assigned to another employee", ErrConflict, shiftID)
	}
	if err := s.checkNoOverlap(shift, employeeID); err != nil {
		return Shift{}, err
	}

	shift.EmployeeID = employeeID
	if err := s.repo.Update(shift); err != nil {
		return Shift{}, err
	}
	return shift, nil
}

// checkNoOverlap returns ErrConflict if the employee already has a shift that
// overlaps the given one.
func (s *Service) checkNoOverlap(shift Shift, employeeID string) error {
	for _, existing := range s.repo.ListByEmployee(employeeID) {
		if existing.Overlaps(shift) {
			return fmt.Errorf("%w: employee %q already has overlapping shift %q", ErrConflict, employeeID, existing.ID)
		}
	}
	return nil
}

func validateTimeRange(start, end time.Time) error {
	if start.IsZero() || end.IsZero() {
		return fmt.Errorf("%w: start time and end time are required", ErrInvalidArgument)
	}
	if !start.Before(end) {
		return fmt.Errorf("%w: start time must be before end time", ErrInvalidArgument)
	}
	return nil
}

// normalizeID trims surrounding whitespace so that "emp-1" and " emp-1 " refer
// to the same employee (otherwise padding could bypass the overlap rule).
// It returns ErrInvalidArgument if nothing is left.
func normalizeID(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInvalidArgument, name)
	}
	return value, nil
}
