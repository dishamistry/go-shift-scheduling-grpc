// Package scheduling contains the shift scheduling domain model and business rules.
//
// It has no knowledge of gRPC or protobuf; the transport layer converts between
// the generated API types and the types in this package.
package scheduling

import "time"

// Shift is a block of work time that can be assigned to one employee.
//
// Shift only holds value types, so copies are independent and can be returned
// to callers without exposing the repository's internal state.
type Shift struct {
	ID         string
	EmployeeID string // empty when the shift is unassigned
	Start      time.Time
	End        time.Time
}

// IsAssigned reports whether an employee is assigned to the shift.
func (s Shift) IsAssigned() bool {
	return s.EmployeeID != ""
}

// Overlaps reports whether two shifts share any moment in time.
//
// Shifts are half-open intervals [Start, End): a shift ending at 10:00 does not
// overlap one starting at 10:00.
func (s Shift) Overlaps(other Shift) bool {
	return s.Start.Before(other.End) && other.Start.Before(s.End)
}
