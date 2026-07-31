package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestFailureClassificationPreservesCauseAndSentinel(t *testing.T) {
	t.Parallel()

	cause := errors.New("adapter detail")
	classified := NewFailure(FailureBusy, "write entry", cause)
	wrapped := fmt.Errorf("service: %w", classified)

	if !errors.Is(wrapped, ErrBusy) {
		t.Fatalf("errors.Is(%v, ErrBusy) = false", wrapped)
	}
	if !errors.Is(wrapped, cause) {
		t.Fatalf("errors.Is(%v, cause) = false", wrapped)
	}
	if got := Classify(wrapped); got != FailureBusy {
		t.Fatalf("Classify = %q, want %q", got, FailureBusy)
	}
}

func TestFailureClassificationLeavesContextErrorsUnclassified(t *testing.T) {
	t.Parallel()

	for _, err := range []error{nil, context.Canceled, context.DeadlineExceeded} {
		if got := Classify(err); got != FailureUnknown {
			t.Errorf("Classify(%v) = %q, want FailureUnknown", err, got)
		}
	}
}
