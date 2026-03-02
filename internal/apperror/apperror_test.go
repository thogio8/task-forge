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
