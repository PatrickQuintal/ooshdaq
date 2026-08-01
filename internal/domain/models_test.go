package domain

import (
	"errors"
	"testing"
)

func TestCollectionEntryValidate(t *testing.T) {
	t.Parallel()

	valid := CollectionEntry{
		UserID:    "user-example",
		ItemID:    "disney-2026-06-elsa",
		Owned:     2,
		Available: 1,
	}

	tests := []struct {
		name   string
		change func(*CollectionEntry)
		field  string
	}{
		{name: "negative owned", change: func(e *CollectionEntry) { e.Owned = -1 }, field: "owned"},
		{name: "negative available", change: func(e *CollectionEntry) { e.Available = -1 }, field: "available"},
		{name: "negative wanted", change: func(e *CollectionEntry) { e.Wanted = -1 }, field: "wanted"},
		{name: "available exceeds owned", change: func(e *CollectionEntry) { e.Available = 3 }, field: "available"},
		{name: "wanted while available", change: func(e *CollectionEntry) { e.Wanted = 1 }, field: "wanted"},
		{name: "owned too large", change: func(e *CollectionEntry) { e.Owned = MaxQuantity + 1 }, field: "owned"},
		{name: "missing user", change: func(e *CollectionEntry) { e.UserID = "" }, field: "user_id"},
		{name: "invalid item", change: func(e *CollectionEntry) { e.ItemID = "../elsa" }, field: "item_id"},
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}
	ownedAndWanted := valid
	ownedAndWanted.Available = 0
	ownedAndWanted.Wanted = 3
	if err := ownedAndWanted.Validate(); err != nil {
		t.Fatalf("valid owned-and-wanted entry rejected: %v", err)
	}
	wantedOnly := valid
	wantedOnly.Owned = 0
	wantedOnly.Available = 0
	wantedOnly.Wanted = 3
	if err := wantedOnly.Validate(); err != nil {
		t.Fatalf("valid wanted-only entry rejected: %v", err)
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entry := valid
			tt.change(&entry)

			var validationError ValidationError
			if err := entry.Validate(); !errors.As(err, &validationError) {
				t.Fatalf("expected ValidationError, got %T (%v)", err, err)
			} else if validationError.Field != tt.field {
				t.Fatalf("field = %q, want %q", validationError.Field, tt.field)
			}
		})
	}
}

func TestUserValidate(t *testing.T) {
	t.Parallel()

	for _, user := range []User{
		{ID: "mock-alex", DisplayName: "Alex Example"},
		{ID: "idp:opaque-subject@issuer", DisplayName: "Example User"},
	} {
		if err := user.Validate(); err != nil {
			t.Errorf("valid user %#v rejected: %v", user, err)
		}
	}

	for _, user := range []User{
		{ID: "../admin", DisplayName: "Example User"},
		{ID: "has space", DisplayName: "Example User"},
		{ID: "example", DisplayName: ""},
		{ID: "example", DisplayName: "line\nbreak"},
	} {
		if err := user.Validate(); err == nil {
			t.Errorf("invalid user %#v accepted", user)
		}
	}
}
