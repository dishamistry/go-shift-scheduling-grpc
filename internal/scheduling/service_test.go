package scheduling

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// at returns 2026-11-02 at the given hour (UTC).
func at(hour int) time.Time {
	return time.Date(2026, 11, 2, hour, 0, 0, 0, time.UTC)
}

func newTestService() *Service {
	return NewService(NewMemoryRepository())
}

func mustCreate(t *testing.T, svc *Service, start, end time.Time) Shift {
	t.Helper()
	s, err := svc.CreateShift(start, end)
	if err != nil {
		t.Fatalf("CreateShift(%v, %v): %v", start, end, err)
	}
	return s
}

func TestCreateShift(t *testing.T) {
	svc := newTestService()

	got := mustCreate(t, svc, at(9), at(17))

	if got.ID == "" {
		t.Error("expected a generated ID")
	}
	if got.IsAssigned() {
		t.Errorf("new shift should be unassigned, got employee %q", got.EmployeeID)
	}
	if !got.Start.Equal(at(9)) || !got.End.Equal(at(17)) {
		t.Errorf("got times %v-%v, want %v-%v", got.Start, got.End, at(9), at(17))
	}
}

func TestCreateShiftConvertsToUTC(t *testing.T) {
	svc := newTestService()
	sydney := time.FixedZone("AEDT", 11*60*60)

	got := mustCreate(t, svc, at(9).In(sydney), at(17).In(sydney))

	if got.Start.Location() != time.UTC || got.End.Location() != time.UTC {
		t.Errorf("expected UTC times, got %v and %v", got.Start.Location(), got.End.Location())
	}
}

func TestCreateShiftGeneratesUniqueIDs(t *testing.T) {
	svc := newTestService()

	a := mustCreate(t, svc, at(9), at(10))
	b := mustCreate(t, svc, at(9), at(10))

	if a.ID == b.ID {
		t.Errorf("expected unique IDs, both were %q", a.ID)
	}
}

func TestCreateShiftInvalidTimes(t *testing.T) {
	tests := []struct {
		name       string
		start, end time.Time
	}{
		{"end before start", at(17), at(9)},
		{"zero length", at(9), at(9)},
		{"missing start", time.Time{}, at(9)},
		{"missing end", at(9), time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newTestService().CreateShift(tt.start, tt.end)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Errorf("got error %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func TestGetShift(t *testing.T) {
	svc := newTestService()
	created := mustCreate(t, svc, at(9), at(17))

	got, err := svc.GetShift(created.ID)
	if err != nil {
		t.Fatalf("GetShift: %v", err)
	}
	if got != created {
		t.Errorf("got %+v, want %+v", got, created)
	}
}

func TestGetShiftErrors(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{"empty id", "", ErrInvalidArgument},
		{"blank id", "   ", ErrInvalidArgument},
		{"unknown id", "does-not-exist", ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newTestService().GetShift(tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAssignEmployee(t *testing.T) {
	svc := newTestService()
	created := mustCreate(t, svc, at(9), at(17))

	got, err := svc.AssignEmployee(created.ID, "emp-1")
	if err != nil {
		t.Fatalf("AssignEmployee: %v", err)
	}
	if got.EmployeeID != "emp-1" {
		t.Errorf("got employee %q, want %q", got.EmployeeID, "emp-1")
	}

	stored, err := svc.GetShift(created.ID)
	if err != nil {
		t.Fatalf("GetShift: %v", err)
	}
	if stored.EmployeeID != "emp-1" {
		t.Errorf("assignment was not saved: got employee %q", stored.EmployeeID)
	}
}

func TestAssignEmployeeSameEmployeeTwiceIsAllowed(t *testing.T) {
	svc := newTestService()
	created := mustCreate(t, svc, at(9), at(17))

	if _, err := svc.AssignEmployee(created.ID, "emp-1"); err != nil {
		t.Fatalf("first assign: %v", err)
	}
	if _, err := svc.AssignEmployee(created.ID, "emp-1"); err != nil {
		t.Errorf("second assign of the same employee: got %v, want nil", err)
	}
}

func TestAssignEmployeeErrors(t *testing.T) {
	svc := newTestService()
	shift := mustCreate(t, svc, at(9), at(17))

	tests := []struct {
		name       string
		shiftID    string
		employeeID string
		wantErr    error
	}{
		{"empty shift id", "", "emp-1", ErrInvalidArgument},
		{"empty employee id", shift.ID, "", ErrInvalidArgument},
		{"blank employee id", shift.ID, "  ", ErrInvalidArgument},
		{"unknown shift", "does-not-exist", "emp-1", ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AssignEmployee(tt.shiftID, tt.employeeID)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAssignEmployeeAlreadyAssignedToSomeoneElse(t *testing.T) {
	svc := newTestService()
	shift := mustCreate(t, svc, at(9), at(17))

	if _, err := svc.AssignEmployee(shift.ID, "emp-1"); err != nil {
		t.Fatalf("first assign: %v", err)
	}
	_, err := svc.AssignEmployee(shift.ID, "emp-2")
	if !errors.Is(err, ErrConflict) {
		t.Errorf("got error %v, want ErrConflict", err)
	}
}

func TestAssignEmployeeOverlap(t *testing.T) {
	tests := []struct {
		name         string
		start, end   time.Time
		wantConflict bool
	}{
		{"identical times", at(9), at(17), true},
		{"starts during existing", at(12), at(20), true},
		{"ends during existing", at(6), at(10), true},
		{"inside existing", at(10), at(11), true},
		{"contains existing", at(8), at(18), true},
		{"ends exactly when existing starts", at(7), at(9), false},
		{"starts exactly when existing ends", at(17), at(20), false},
		{"completely separate", at(18), at(22), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			existing := mustCreate(t, svc, at(9), at(17))
			if _, err := svc.AssignEmployee(existing.ID, "emp-1"); err != nil {
				t.Fatalf("assign existing shift: %v", err)
			}
			candidate := mustCreate(t, svc, tt.start, tt.end)

			_, err := svc.AssignEmployee(candidate.ID, "emp-1")

			if tt.wantConflict && !errors.Is(err, ErrConflict) {
				t.Errorf("got error %v, want ErrConflict", err)
			}
			if !tt.wantConflict && err != nil {
				t.Errorf("got error %v, want nil", err)
			}
		})
	}
}

func TestAssignEmployeeOverlapOnlyAppliesToSameEmployee(t *testing.T) {
	svc := newTestService()
	a := mustCreate(t, svc, at(9), at(17))
	b := mustCreate(t, svc, at(9), at(17))

	if _, err := svc.AssignEmployee(a.ID, "emp-1"); err != nil {
		t.Fatalf("assign emp-1: %v", err)
	}
	if _, err := svc.AssignEmployee(b.ID, "emp-2"); err != nil {
		t.Errorf("different employees may work at the same time: got %v", err)
	}
}

// Many goroutines try to assign the same employee to overlapping shifts at once.
// Exactly one assignment must succeed.
func TestAssignEmployeeConcurrentOverlap(t *testing.T) {
	svc := newTestService()
	const n = 20
	ids := make([]string, n)
	for i := range ids {
		ids[i] = mustCreate(t, svc, at(9), at(17)).ID
	}

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.AssignEmployee(id, "emp-1")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	succeeded := 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, ErrConflict):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("%d assignments succeeded, want exactly 1", succeeded)
	}
}
