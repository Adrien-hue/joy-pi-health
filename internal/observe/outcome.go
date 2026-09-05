// Package observe owns metric semantics and typed collection outcomes.
package observe

import (
	"errors"
	"io/fs"
)

type availabilityReason uint8

const (
	reasonUnsupported availabilityReason = iota + 1
	reasonPermissionDenied
	reasonTemporarilyUnavailable
)

type outcomeState uint8

const (
	statePresent outcomeState = iota + 1
	stateUnavailable
	stateDefect
)

type outcome[T any] struct {
	state  outcomeState
	value  T
	reason availabilityReason
	cause  error
}

func present[T any](value T) outcome[T] {
	return outcome[T]{state: statePresent, value: value}
}

func unavailable[T any](reason availabilityReason, cause error) outcome[T] {
	if reason < reasonUnsupported || reason > reasonTemporarilyUnavailable || cause == nil {
		return defect[T](errors.New("invalid unavailable outcome"))
	}
	return outcome[T]{state: stateUnavailable, reason: reason, cause: cause}
}

func defect[T any](cause error) outcome[T] {
	if cause == nil {
		cause = errors.New("unspecified internal defect")
	}
	return outcome[T]{state: stateDefect, cause: cause}
}

func acquisitionFailure[T any](err error, missing availabilityReason) outcome[T] {
	switch {
	case err == nil:
		return defect[T](errors.New("acquisition failure has no cause"))
	case errors.Is(err, fs.ErrPermission):
		return unavailable[T](reasonPermissionDenied, err)
	case errors.Is(err, fs.ErrNotExist):
		return unavailable[T](missing, err)
	default:
		return unavailable[T](reasonTemporarilyUnavailable, err)
	}
}
