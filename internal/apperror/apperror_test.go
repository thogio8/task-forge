package apperror

import (
	"errors"
	"fmt"
	"testing"
)

func TestNotFoundWrapping(t *testing.T) {
	msg := "task not found"
	notFound := NotFound(msg, nil)
	var target *NotFoundError

	err := fmt.Errorf("repository: %w", notFound)

	if !errors.As(err, &target) {
		t.Errorf("errors.As did not match *NotFoundError")
	}

	if target.Message != "task not found" {
		t.Errorf("got %v, want %v", target.Message, msg)
	}
}

func TestValidationWrapping(t *testing.T) {
	msg := "invalid task payload"
	validation := Validation(msg, nil)
	var target *ValidationError

	err := fmt.Errorf("repository: %w", validation)

	if !errors.As(err, &target) {
		t.Errorf("errors.As did not match *ValidationError")
	}

	if target.Message != msg {
		t.Errorf("got %v, want %v", target.Message, msg)
	}
}

func TestInternalWrapping(t *testing.T) {
	msg := "internal server error"
	internal := Internal(msg, nil)
	var target *InternalError

	err := fmt.Errorf("repository: %w", internal)

	if !errors.As(err, &target) {
		t.Errorf("errors.As did not match *InternalError")
	}

	if target.Message != "internal server error" {
		t.Errorf("got %v, want %v", target.Message, msg)
	}
}

// TestUnwrapPropagatesSentinel verifies that the three error types correctly
// implement Unwrap, so callers can use errors.Is to match against sentinel
// causes. Without Unwrap, errors.Is would only walk the wrapping chain via
// fmt.Errorf %w, never reaching the inner cause set at construction.
func TestUnwrapPropagatesSentinel(t *testing.T) {
	sentinel := errors.New("db unavailable")

	tests := []struct {
		name string
		err  error
	}{
		{"NotFound", NotFound("not found", sentinel)},
		{"Validation", Validation("invalid", sentinel)},
		{"Internal", Internal("internal", sentinel)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, sentinel) {
				t.Errorf("errors.Is(%T, sentinel) failed — Unwrap likely broken", tt.err)
			}
		})
	}
}

// TestErrorMessageMatchesConstructorInput verifies the Error() string
// returns the message passed at construction (not derived from the inner
// cause). This is the contract callers rely on for log lines and HTTP body.
func TestErrorMessageMatchesConstructorInput(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"NotFound", NotFound("not found msg", nil)},
		{"Validation", Validation("validation msg", nil)},
		{"Internal", Internal("internal msg", nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := map[string]string{
				"NotFound":   "not found msg",
				"Validation": "validation msg",
				"Internal":   "internal msg",
			}[tt.name]
			if got := tt.err.Error(); got != want {
				t.Errorf("Error() = %q, want %q", got, want)
			}
		})
	}
}
