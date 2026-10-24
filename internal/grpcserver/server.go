// Package grpcserver implements the ShiftService gRPC API.
//
// It is a thin transport layer: each handler validates the protobuf request,
// converts it to domain values, calls the scheduling service, and converts the
// result (or error) back into a protobuf response (or gRPC status).
package grpcserver

import (
	"context"

	shiftv1 "github.com/dishamistry/go-shift-scheduling-grpc/gen/shift/v1"
	"github.com/dishamistry/go-shift-scheduling-grpc/internal/scheduling"
)

// Server implements shiftv1.ShiftServiceServer on top of a scheduling.Service.
type Server struct {
	// Embedding by value is required by the generated code. It makes any RPC
	// added to the .proto later return codes.Unimplemented until it is written.
	shiftv1.UnimplementedShiftServiceServer

	svc *scheduling.Service
}

// NewServer returns a Server that delegates business logic to svc.
func NewServer(svc *scheduling.Service) *Server {
	return &Server{svc: svc}
}

// CreateShift creates a new, unassigned shift.
func (s *Server) CreateShift(ctx context.Context, req *shiftv1.CreateShiftRequest) (*shiftv1.CreateShiftResponse, error) {
	start, err := requiredTime("start_time", req.GetStartTime())
	if err != nil {
		return nil, err
	}
	end, err := requiredTime("end_time", req.GetEndTime())
	if err != nil {
		return nil, err
	}

	shift, err := s.svc.CreateShift(start, end)
	if err != nil {
		return nil, toStatus(err)
	}
	return &shiftv1.CreateShiftResponse{Shift: toProto(shift)}, nil
}

// GetShift returns a shift by ID.
func (s *Server) GetShift(ctx context.Context, req *shiftv1.GetShiftRequest) (*shiftv1.GetShiftResponse, error) {
	if err := requiredString("id", req.GetId()); err != nil {
		return nil, err
	}

	shift, err := s.svc.GetShift(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &shiftv1.GetShiftResponse{Shift: toProto(shift)}, nil
}

// AssignEmployee assigns an employee to an existing shift.
func (s *Server) AssignEmployee(ctx context.Context, req *shiftv1.AssignEmployeeRequest) (*shiftv1.AssignEmployeeResponse, error) {
	if err := requiredString("shift_id", req.GetShiftId()); err != nil {
		return nil, err
	}
	if err := requiredString("employee_id", req.GetEmployeeId()); err != nil {
		return nil, err
	}

	shift, err := s.svc.AssignEmployee(req.GetShiftId(), req.GetEmployeeId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &shiftv1.AssignEmployeeResponse{Shift: toProto(shift)}, nil
}
