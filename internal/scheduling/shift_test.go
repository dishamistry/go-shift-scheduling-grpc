package scheduling

import (
	"testing"
	"time"
)

func TestShiftOverlaps(t *testing.T) {
	existing := Shift{Start: at(9), End: at(17)}

	tests := []struct {
		name       string
		start, end time.Time
		want       bool
	}{
		{"identical", at(9), at(17), true},
		{"starts inside", at(12), at(20), true},
		{"ends inside", at(6), at(10), true},
		{"contained", at(10), at(11), true},
		{"contains", at(8), at(18), true},
		{"overlaps by one nanosecond", at(6), at(9).Add(time.Nanosecond), true},
		{"adjacent before (ends at start)", at(7), at(9), false},
		{"adjacent after (starts at end)", at(17), at(20), false},
		{"separate", at(18), at(22), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := Shift{Start: tt.start, End: tt.end}

			// Overlap must be symmetric: check both directions.
			if got := existing.Overlaps(other); got != tt.want {
				t.Errorf("existing.Overlaps(other) = %v, want %v", got, tt.want)
			}
			if got := other.Overlaps(existing); got != tt.want {
				t.Errorf("other.Overlaps(existing) = %v, want %v", got, tt.want)
			}
		})
	}
}
