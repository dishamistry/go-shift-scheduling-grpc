package grpcserver

import (
	"errors"
	"log"
	"strings"
	"time"

	shiftv1 "github.com/dishamistry/go-shift-scheduling-grpc/gen/shift/v1"
	"github.com/dishamistry/go-shift-scheduling-grpc/internal/scheduling"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// requiredString returns an InvalidArgument status if value is empty or blank.
func requiredString(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return status.Errorf(codes.InvalidArgument, "%s is required", field)
	}
	return nil
}

// requiredTime converts a protobuf Timestamp to time.Time. It returns an
// InvalidArgument status if the timestamp is missing or out of range
// (for example, nanos outside 0..999,999,999 or a year outside 1..9999).
func requiredTime(field string, ts *timestamppb.Timestamp) (time.Time, error) {
	if ts == nil {
		return time.Time{}, status.Errorf(codes.InvalidArgument, "%s is required", field)
	}
	if err := ts.CheckValid(); err != nil {
		return time.Time{}, status.Errorf(codes.InvalidArgument, "%s is not a valid timestamp", field)
	}
	return ts.AsTime(), nil
}

// toProto converts a domain shift to its protobuf representation.
func toProto(s scheduling.Shift) *shiftv1.Shift {
	return &shiftv1.Shift{
		Id:         s.ID,
		EmployeeId: s.EmployeeID,
		StartTime:  timestamppb.New(s.Start),
		EndTime:    timestamppb.New(s.End),
	}
}

// toStatus maps a domain error to a gRPC status error.
//
// Scheduling conflicts map to FailedPrecondition rather than AlreadyExists:
// the request is valid, but the current state of the system (an overlapping
// shift, or the shift already belonging to someone else) does not allow it.
// AlreadyExists is meant for creating an entity that already exists, which
// is not what these conflicts are.
//
// Domain error messages are written for API callers, so they are returned as is.
// Any other error is logged and replaced by a generic Internal status so that
// implementation details are not exposed to clients.
func toStatus(err error) error {
	switch {
	case errors.Is(err, scheduling.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, scheduling.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, scheduling.ErrConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		log.Printf("internal error: %v", err)
		return status.Error(codes.Internal, "internal error")
	}
}
