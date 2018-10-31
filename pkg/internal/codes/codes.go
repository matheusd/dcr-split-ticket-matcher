package codes

// Syntatic sugar helpers for grpc error constructors

import (
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Code is an unsigned 32 bit integer that corresponds to a GRPC error code
type Code uint32

// The following constants all correspond to their values in the
// google.golang.org/grpc/codes package.
const (
	OK                 Code = Code(codes.OK)
	Canceled           Code = Code(codes.Canceled)
	Unknown            Code = Code(codes.Unknown)
	InvalidArgument    Code = Code(codes.InvalidArgument)
	DeadlineExceeded   Code = Code(codes.DeadlineExceeded)
	NotFound           Code = Code(codes.NotFound)
	AlreadyExists      Code = Code(codes.AlreadyExists)
	PermissionDenied   Code = Code(codes.PermissionDenied)
	ResourceExhausted  Code = Code(codes.ResourceExhausted)
	FailedPrecondition Code = Code(codes.FailedPrecondition)
	Aborted            Code = Code(codes.Aborted)
	OutOfRange         Code = Code(codes.OutOfRange)
	Unimplemented      Code = Code(codes.Unimplemented)
	Internal           Code = Code(codes.Internal)
	Unavailable        Code = Code(codes.Unavailable)
	DataLoss           Code = Code(codes.DataLoss)
	Unauthenticated    Code = Code(codes.Unauthenticated)
)

// Wrap wraps e with the given message and returns a status error with the given
// code.
func (c Code) Wrap(e error, msg string) error {
	if (c == OK) && ((e != nil) || (msg != "")) {
		panic("Wrong usage: cannot wrap OK status with an error msg")
	}
	return status.Error(codes.Code(c), errors.Wrap(e, msg).Error())
}

// Wrapf wraps e with the given message and arguments and returns a status error
// with the given code.
func (c Code) Wrapf(e error, msg string, args ...interface{}) error {
	if (c == OK) && ((e != nil) || (msg != "")) {
		panic("Wrong usage: cannot wrapf OK status with an error msg")
	}
	return status.Error(codes.Code(c), errors.Wrapf(e, msg, args...).Error())
}

// Error returns a grpc status error with the given code and message
func (c Code) Error(msg string) error {
	if (c == OK) && (msg != "") {
		panic("Wrong usage: cannot error OK status with an error msg")
	}
	return status.Error(codes.Code(c), msg)
}

// Errorf returns a grpc status error with the given code, message and args
func (c Code) Errorf(msg string, args ...interface{}) error {
	if (c == OK) && (msg != "") {
		panic("Wrong usage: cannot error OK status with an error msg")
	}
	return status.Errorf(codes.Code(c), msg, args...)
}
