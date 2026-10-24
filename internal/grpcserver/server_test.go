package grpcserver

import (
	"context"
	"errors"
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
	"google.golang.org/protobuf/types/known/timestamppb"
)

// newTestClient starts a real gRPC server over an in-memory connection and
// returns a client for it. Requests go through full gRPC encoding and decoding.
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
		t.Fatalf("CreateShift: %v", err)
	}
	return resp.GetShift()
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Errorf("got code %v, want %v (err: %v)", got, want, err)
	}
}

func TestCreateGetAndAssign(t *testing.T) {
	client := newTestClient(t)
	ctx := testContext(t)

	created := createShift(t, client, 9, 17)
	if created.GetId() == "" {
		t.Fatal("expected a generated ID")
	}
	if created.GetEmployeeId() != "" {
		t.Errorf("new shift should be unassigned, got %q", created.GetEmployeeId())
	}
	if !created.GetStartTime().AsTime().Equal(ts(9).AsTime()) || !created.GetEndTime().AsTime().Equal(ts(17).AsTime()) {
		t.Errorf("unexpected times: %v - %v", created.GetStartTime(), created.GetEndTime())
	}

	assigned, err := client.AssignEmployee(ctx, &shiftv1.AssignEmployeeRequest{ShiftId: created.GetId(), EmployeeId: "emp-1"})
	if err != nil {
		t.Fatalf("AssignEmployee: %v", err)
	}
	if assigned.GetShift().GetEmployeeId() != "emp-1" {
		t.Errorf("got employee %q, want emp-1", assigned.GetShift().GetEmployeeId())
	}

	got, err := client.GetShift(ctx, &shiftv1.GetShiftRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("GetShift: %v", err)
	}
	if got.GetShift().GetEmployeeId() != "emp-1" {
		t.Errorf("GetShift returned employee %q, want emp-1", got.GetShift().GetEmployeeId())
	}
}

func TestCreateShiftInvalidArgument(t *testing.T) {
	client := newTestClient(t)

	tests := []struct {
		name string
		req  *shiftv1.CreateShiftRequest
	}{
		{"missing start", &shiftv1.CreateShiftRequest{EndTime: ts(17)}},
		{"missing end", &shiftv1.CreateShiftRequest{StartTime: ts(9)}},
		{"invalid timestamp", &shiftv1.CreateShiftRequest{StartTime: &timestamppb.Timestamp{Nanos: -1}, EndTime: ts(17)}},
		{"end before start", &shiftv1.CreateShiftRequest{StartTime: ts(17), EndTime: ts(9)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateShift(testContext(t), tt.req)
			wantCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestGetShiftErrors(t *testing.T) {
	client := newTestClient(t)

	_, err := client.GetShift(testContext(t), &shiftv1.GetShiftRequest{Id: ""})
	wantCode(t, err, codes.InvalidArgument)

	_, err = client.GetShift(testContext(t), &shiftv1.GetShiftRequest{Id: "does-not-exist"})
	wantCode(t, err, codes.NotFound)
}

func TestAssignEmployeeErrors(t *testing.T) {
	client := newTestClient(t)
	shift := createShift(t, client, 9, 17)

	tests := []struct {
		name string
		req  *shiftv1.AssignEmployeeRequest
		want codes.Code
	}{
		{"missing shift id", &shiftv1.AssignEmployeeRequest{EmployeeId: "emp-1"}, codes.InvalidArgument},
		{"missing employee id", &shiftv1.AssignEmployeeRequest{ShiftId: shift.GetId()}, codes.InvalidArgument},
		{"unknown shift", &shiftv1.AssignEmployeeRequest{ShiftId: "does-not-exist", EmployeeId: "emp-1"}, codes.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.AssignEmployee(testContext(t), tt.req)
			wantCode(t, err, tt.want)
		})
	}
}

func TestAssignEmployeeConflictIsFailedPrecondition(t *testing.T) {
	client := newTestClient(t)
	ctx := testContext(t)
	first := createShift(t, client, 9, 17)
	overlapping := createShift(t, client, 12, 20)

	if _, err := client.AssignEmployee(ctx, &shiftv1.AssignEmployeeRequest{ShiftId: first.GetId(), EmployeeId: "emp-1"}); err != nil {
		t.Fatalf("first assign: %v", err)
	}

	_, err := client.AssignEmployee(ctx, &shiftv1.AssignEmployeeRequest{ShiftId: overlapping.GetId(), EmployeeId: "emp-1"})
	wantCode(t, err, codes.FailedPrecondition)

	_, err = client.AssignEmployee(ctx, &shiftv1.AssignEmployeeRequest{ShiftId: first.GetId(), EmployeeId: "emp-2"})
	wantCode(t, err, codes.FailedPrecondition)
}

func TestToStatusHidesUnexpectedErrors(t *testing.T) {
	err := toStatus(errors.New("disk on fire at /var/lib/secret"))

	wantCode(t, err, codes.Internal)
	if msg := status.Convert(err).Message(); strings.Contains(msg, "secret") {
		t.Errorf("internal details leaked to client: %q", msg)
	}
}
