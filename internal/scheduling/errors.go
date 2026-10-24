package scheduling

import "errors"

// Sentinel errors returned by this package. Errors are wrapped with extra
// detail, so callers should check them with errors.Is, for example
// errors.Is(err, ErrNotFound). The gRPC layer maps each one to a status code.
var (
	// ErrInvalidArgument means the caller sent invalid input.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrNotFound means the requested shift does not exist.
	ErrNotFound = errors.New("shift not found")

	// ErrConflict means the request breaks a scheduling rule given the current
	// state, such as assigning an employee to overlapping shifts.
	ErrConflict = errors.New("scheduling conflict")
)
