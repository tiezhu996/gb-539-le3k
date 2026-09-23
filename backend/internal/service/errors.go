package service

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrForbidden = errors.New("forbidden")
var ErrValidation = errors.New("validation")

// ErrMissingFrozenPlan is returned when a lot tries to leave equalizing
// without a frozen, non-expired schedule for the current measurement basis.
// It wraps ErrConflict so it is served as 409 while carrying its own code.
var ErrMissingFrozenPlan = fmt.Errorf("%w: missing frozen current plan", ErrConflict)

// ConflictCode classifies a conflict so handlers can return a stable machine
// code in addition to the HTTP 409 status.
func ConflictCode(err error) string {
	if errors.Is(err, ErrMissingFrozenPlan) {
		return "missing_plan"
	}
	if errors.Is(err, ErrConflict) {
		return "conflict"
	}
	return ""
}
