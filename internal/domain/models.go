// Package domain contains the business objects and rules used by $OOSHDAQ.
// It intentionally has no dependency on HTTP or a particular database.
package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxQuantity prevents accidental or malicious integer-sized inventories while
	// remaining far above any plausible personal collection.
	MaxQuantity = 1_000_000
)

// User is a locally persisted profile. Authentication credentials and identity
// provider claims do not belong in this type.
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Validate verifies that a user can be safely addressed and displayed.
func (u User) Validate() error {
	if !validUserID(u.ID) {
		return ValidationError{
			Field:  "id",
			Reason: "must be 1-200 visible characters without spaces, slashes, or control characters",
		}
	}
	displayName := strings.TrimSpace(u.DisplayName)
	if displayName == "" || len(displayName) > 100 || !utf8.ValidString(displayName) {
		return ValidationError{Field: "display_name", Reason: "must be between 1 and 100 characters"}
	}
	for _, r := range displayName {
		if unicode.IsControl(r) {
			return ValidationError{Field: "display_name", Reason: "must not contain control characters"}
		}
	}
	return nil
}

// CollectionEntry records a user's quantities for one globally unique catalog
// item.
//
// Wanted is a quantity rather than a boolean so collection and match views can
// preserve the member's actual demand.
type CollectionEntry struct {
	UserID    string `json:"user_id"`
	ItemID    string `json:"item_id"`
	Owned     int    `json:"owned"`
	Available int    `json:"available"`
	Wanted    int    `json:"wanted"`
}

// Validate enforces the inventory rules independently of persistence.
func (e CollectionEntry) Validate() error {
	if !validUserID(e.UserID) {
		return ValidationError{Field: "user_id", Reason: "is invalid"}
	}
	if !validSlug(e.ItemID) {
		return ValidationError{Field: "item_id", Reason: "is invalid"}
	}
	for _, field := range []struct {
		name     string
		quantity int
	}{
		{name: "owned", quantity: e.Owned},
		{name: "available", quantity: e.Available},
		{name: "wanted", quantity: e.Wanted},
	} {
		if field.quantity < 0 {
			return ValidationError{Field: field.name, Reason: "must be nonnegative"}
		}
		if field.quantity > MaxQuantity {
			return ValidationError{
				Field:  field.name,
				Reason: fmt.Sprintf("must not exceed %d", MaxQuantity),
			}
		}
	}
	if e.Available > e.Owned {
		return ValidationError{Field: "available", Reason: "must not exceed owned"}
	}
	if e.Wanted > 0 && e.Available > 0 {
		return ValidationError{
			Field:  "wanted",
			Reason: "must be zero when available is positive",
		}
	}
	return nil
}

func validSlug(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' && i > 0:
		default:
			return false
		}
	}
	return value[len(value)-1] != '-'
}

func validUserID(value string) bool {
	if value == "" || len(value) > 200 || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}
