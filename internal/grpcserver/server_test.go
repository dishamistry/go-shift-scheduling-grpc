package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	shiftv1 "github.com/dishamistry/go-shift-scheduling-grpc/gen/shift/v1"
	"github.com/dishamistry/go-shift-scheduling-grpc/internal/scheduling"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// newTestClient starts a real gRPC server over an in-memory connection and
// returns a client for it. Requests go through full gRPC encoding and decoding,
// but no network port is used.
//
// Every call creates a new server with its own empty storage, so tests never
// share state.
func newTestClient(t *testing.T) shiftv1.ShiftServiceClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	svc := scheduling.NewService(scheduling.NewMemoryRepository())
	shiftv1.RegisterShiftServiceServer(srv, NewServer(svc))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return shiftv1.NewShiftServiceClient(conn)
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// ts returns 2026-11-02 at the given hour (UTC) as a protobuf Timestamp.
func ts(hour int) *timestamppb.Timestamp {
	return timestamppb.New(time.Date(2026, 11, 2, hour, 0, 0, 0, time.UTC))
}

func createShift(t *testing.T, client shiftv1.ShiftServiceClient, startHour, endHour int) *shiftv1.Shift {
	t.Helper()
	resp, err := client.CreateShift(testContext(t), &shiftv1.CreateShiftRequest{
		StartTime: ts(startHour),
		EndTime:   ts(endHour),
	})
	if err != nil {
		t.Fatalf("CreateShift(%d, %d): %v", startHour, endHour, err)
	}
	return resp.GetShift()
}

func assign(client shiftv1.ShiftServiceClient, ctx context.Context, shiftID, employeeID string) (*shiftv1.AssignEmployeeResponse, error) {
	return client.AssignEmployee(ctx, &shiftv1.AssignEmployeeRequest{ShiftId: shiftID, EmployeeId: employeeID})
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Errorf("got code %v, want %v (err: %v)", got, want, err)
	}
}

func TestCreateShift(t *testing.T) {
	client := newTestClient(t)

	got := createShift(t, client, 9, 17)

	if got.GetId() == "" {
		t.Error("expected a generated ID")
	}
	if got.GetEmployeeId() != "" {
		t.Errorf("new shift should be unassigned, got %q", got.GetEmployeeId())
	}
	if !proto.Equal(got.GetStartTime(), ts(9)) || !proto.Equal(got.GetEndTime(), ts(17)) {
		t.Errorf("got times %v - %v, want %v - %v", got.GetStartTime(), got.GetEndTime(), ts(9), ts(17))
	}
}

func TestGetShiftReturnsCreatedShift(t *testing.T) {
	client := newTestClient(t)
	created := createShift(t, client, 9, 17)

	resp, err := client.GetShift(testContext(t), &shiftv1.GetShiftRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("GetShift: %v", err)
	}
	if !proto.Equal(resp.GetShift(), created) {
		t.Errorf("got %v, want %v", resp.GetShift(), created)
	}
}

func TestAssignEmployee(t *testing.T) {
	client := newTestClient(t)
	ctx := testContext(t)
	created := createShift(t, client, 9, 17)

	resp, err := assign(client, ctx, created.GetId(), "emp-1")
	if err != nil {
		t.Fatalf("AssignEmployee: %v", err)
	}
	if resp.GetShift().GetEmployeeId() != "emp-1" {
		t.Errorf("response employee = %q, want emp-1", resp.GetShift().GetEmployeeId())
	}

	got, err := client.GetShift(ctx, &shiftv1.GetShiftRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("GetShift: %v", err)
	}
	if got.GetShift().GetEmployeeId() != "emp-1" {
		t.Errorf("stored employee = %q, want emp-1", got.GetShift().GetEmployeeId())
	}
}

func TestCreateShiftInvalidArgument(t *testing.T) {
	tests := []struct {
		name        string
		req         *shiftv1.CreateShiftRequest
		wantMessage string // substring expected in the status message
	}{
		{"empty request", &shiftv1.CreateShiftRequest{}, "start_time is required"},
		{"missing start", &shiftv1.CreateShiftRequest{EndTime: ts(17)}, "start_time is required"},
		{"missing end", &shiftv1.CreateShiftRequest{StartTime: ts(9)}, "end_time is required"},
		{"negative nanos", &shiftv1.CreateShiftRequest{StartTime: &timestamppb.Timestamp{Nanos: -1}, EndTime: ts(17)}, "start_time is not a valid timestamp"},
		{"year after 9999", &shiftv1.CreateShiftRequest{StartTime: ts(9), EndTime: &timestamppb.Timestamp{Seconds: 253402300800}}, "end_time is not a valid timestamp"},
		{"end before start", &shiftv1.CreateShiftRequest{StartTime: ts(17), EndTime: ts(9)}, "start time must be before end time"},
		{"zero length", &shiftv1.CreateShiftRequest{StartTime: ts(9), EndTime: ts(9)}, "start time must be before end time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newTestClient(t).CreateShift(testContext(t), tt.req)

			wantCode(t, err, codes.InvalidArgument)
			if msg := status.Convert(err).Message(); !strings.Contains(msg, tt.wantMessage) {
				t.Errorf("message %q does not contain %q", msg, tt.wantMessage)
			}
		})
	}
}

func TestGetShiftErrors(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want codes.Code
	}{
		{"empty id", "", codes.InvalidArgument},
		{"blank id", "   ", codes.InvalidArgument},
		{"unknown id", "does-not-exist", codes.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newTestClient(t).GetShift(testContext(t), &shiftv1.GetShiftRequest{Id: tt.id})
			wantCode(t, err, tt.want)
		})
	}
}

func TestAssignEmployeeRequestErrors(t *testing.T) {
	tests := []struct {
		name       string
		useShift   bool // true: send the ID of a real shift
		shiftID    string
		employeeID string
		want       codes.Code
	}{
		{"missing shift id", false, "", "emp-1", codes.InvalidArgument},
		{"missing employee id", true, "", "", codes.InvalidArgument},
		{"blank employee id", true, "", "  ", codes.InvalidArgument},
		{"unknown shift", false, "does-not-exist", "emp-1", codes.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t)
			shiftID := tt.shiftID
			if tt.useShift {
				shiftID = createShift(t, client, 9, 17).GetId()
			}

			_, err := assign(client, testContext(t), shiftID, tt.employeeID)
			wantCode(t, err, tt.want)
		})
	}
}

// Each case starts with emp-1 assigned to 09:00-17:00, then tries a second assignment.
func TestAssignEmployeeSchedulingRules(t *testing.T) {
	tests := []struct {
		name               string
		startHour, endHour int
		employeeID         string
		sameShift          bool // true: assign the existing 09:00-17:00 shift again
		want               codes.Code
	}{
		{"overlapping shift, same employee", 12, 20, "emp-1", false, codes.FailedPrecondition},
		{"shift inside existing, same employee", 10, 11, "emp-1", false, codes.FailedPrecondition},
		{"adjacent shift after, same employee", 17, 20, "emp-1", false, codes.OK},
		{"adjacent shift before, same employee", 6, 9, "emp-1", false, codes.OK},
		{"overlapping shift, different employee", 12, 20, "emp-2", false, codes.OK},
		{"same shift, different employee", 0, 0, "emp-2", true, codes.FailedPrecondition},
		{"same shift, same employee again", 0, 0, "emp-1", true, codes.OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t)
			ctx := testContext(t)
			existing := createShift(t, client, 9, 17)
			if _, err := assign(client, ctx, existing.GetId(), "emp-1"); err != nil {
				t.Fatalf("assign existing shift: %v", err)
			}

			target := existing
			if !tt.sameShift {
				target = createShift(t, client, tt.startHour, tt.endHour)
			}

			resp, err := assign(client, ctx, target.GetId(), tt.employeeID)

			wantCode(t, err, tt.want)
			if tt.want == codes.OK && resp.GetShift().GetEmployeeId() != tt.employeeID {
				t.Errorf("response employee = %q, want %q", resp.GetShift().GetEmployeeId(), tt.employeeID)
			}
		})
	}
}

func TestToStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"invalid argument", fmt.Errorf("%w: bad", scheduling.ErrInvalidArgument), codes.InvalidArgument},
		{"not found", fmt.Errorf("%w: x", scheduling.ErrNotFound), codes.NotFound},
		{"conflict", fmt.Errorf("%w: overlap", scheduling.ErrConflict), codes.FailedPrecondition},
		{"wrapped twice", fmt.Errorf("outer: %w", fmt.Errorf("%w: x", scheduling.ErrNotFound)), codes.NotFound},
		{"unexpected", errors.New("boom"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantCode(t, toStatus(tt.err), tt.want)
		})
	}
}

func TestToStatusHidesUnexpectedErrorDetails(t *testing.T) {
	err := toStatus(errors.New("disk on fire at /var/lib/secret"))

	wantCode(t, err, codes.Internal)
	if msg := status.Convert(err).Message(); msg != "internal error" {
		t.Errorf("message = %q, want %q", msg, "internal error")
	}
}
