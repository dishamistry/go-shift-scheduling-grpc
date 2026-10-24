package scheduling

import (
	"errors"
	"testing"
)

func TestMemoryRepositoryCreateAndGet(t *testing.T) {
	repo := NewMemoryRepository()
	want := Shift{ID: "s1", Start: at(9), End: at(17)}

	if err := repo.Create(want); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.Get("s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMemoryRepositoryCreateDuplicateID(t *testing.T) {
	repo := NewMemoryRepository()
	if err := repo.Create(Shift{ID: "s1"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	err := repo.Create(Shift{ID: "s1"})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("got error %v, want ErrConflict", err)
	}
}

func TestMemoryRepositoryNotFound(t *testing.T) {
	repo := NewMemoryRepository()

	if _, err := repo.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get: got error %v, want ErrNotFound", err)
	}
	if err := repo.Update(Shift{ID: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update: got error %v, want ErrNotFound", err)
	}
}

// Callers receive copies, so changing a returned shift must not change stored data.
// This guards against a future refactor to pointers (for example map[string]*Shift
// or Get returning *Shift), which would let callers bypass the mutex and the
// business rules. Linters may report the write below as unused; that is the point.
func TestMemoryRepositoryReturnsCopies(t *testing.T) {
	repo := NewMemoryRepository()
	if err := repo.Create(Shift{ID: "s1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _ := repo.Get("s1")
	got.EmployeeID = "emp-1"

	stored, _ := repo.Get("s1")
	if stored.EmployeeID != "" {
		t.Errorf("stored shift was modified through a returned copy: %+v", stored)
	}
}

func TestMemoryRepositoryListByEmployee(t *testing.T) {
	repo := NewMemoryRepository()
	for _, s := range []Shift{
		{ID: "a", EmployeeID: "emp-1"},
		{ID: "b", EmployeeID: "emp-2"},
		{ID: "c", EmployeeID: "emp-1"},
		{ID: "d"}, // unassigned
	} {
		if err := repo.Create(s); err != nil {
			t.Fatalf("Create(%q): %v", s.ID, err)
		}
	}

	got := repo.ListByEmployee("emp-1")

	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
	}
	if len(got) != 2 || !ids["a"] || !ids["c"] {
		t.Errorf("got %+v, want shifts a and c", got)
	}
	if len(repo.ListByEmployee("nobody")) != 0 {
		t.Error("expected no shifts for an unknown employee")
	}
}
