package domain

import "fmt"

// ValidationError identifies one invalid input field without exposing internal
// implementation details.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Reason)
}
