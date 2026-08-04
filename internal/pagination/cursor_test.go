package pagination

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func testCodec(t *testing.T) *Codec {
	t.Helper()
	codec, err := New([]byte("0123456789abcdef0123456789abcdef"), 15*time.Minute)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	codec.now = func() time.Time {
		return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	}
	return codec
}

func TestRoundTripBindsRouteSubjectAndFilters(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	input := State{
		Route:   "collection:catalog-one",
		Subject: "member-one",
		Filters: map[string]string{
			"q":         "red",
			"state":     "wanted",
			"universe":  "Example",
			"ranked_at": "2026-07-30T12:00:00Z",
			"readiness": "unfulfillable",
		},
		Limit:    40,
		Position: []string{"0", "1780000000", "1", "member-two", "item-twelve"},
	}
	token, err := codec.Encode(input)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.ContainsAny(token, "+/=") {
		t.Fatalf("token is not raw base64url-safe: %q", token)
	}
	got, err := codec.Decode(token, input.Route, input.Subject)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Limit != input.Limit ||
		got.Filters["state"] != "wanted" ||
		got.Position[4] != "item-twelve" {
		t.Fatalf("decoded state = %#v", got)
	}

	got.Filters["state"] = "owned"
	got.Position[4] = "changed"
	again, err := codec.Decode(token, input.Route, input.Subject)
	if err != nil {
		t.Fatalf("Decode again: %v", err)
	}
	if again.Filters["state"] != "wanted" || again.Position[4] != "item-twelve" {
		t.Fatal("Decode returned state sharing caller-owned storage")
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode opaque token: %v", err)
	}
	for _, marker := range [][]byte{
		[]byte("member-one"),
		[]byte("ranked_at"),
		[]byte("1780000000"),
		[]byte("unfulfillable"),
	} {
		if bytes.Contains(raw, marker) {
			t.Fatalf("opaque cursor contains plaintext marker %q", marker)
		}
	}
}

func TestDecodeRejectsTamperingAndWrongBinding(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	token, err := codec.Encode(State{
		Route:   "matches",
		Subject: "member-one",
		Filters: map[string]string{"scope": "cross_catalog"},
		Limit:   20,
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	tamperedBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode token for tampering: %v", err)
	}
	tamperedBytes[len(tamperedBytes)-1] ^= 1
	tampered := base64.RawURLEncoding.EncodeToString(tamperedBytes)

	for name, candidate := range map[string]struct {
		token   string
		route   string
		subject string
	}{
		"tampered":      {tampered, "matches", "member-one"},
		"wrong route":   {token, "catalogs", "member-one"},
		"wrong subject": {token, "matches", "member-two"},
		"malformed":     {"not-a-cursor", "matches", "member-one"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := codec.Decode(candidate.token, candidate.route, candidate.subject)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Decode error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestDecodeRejectsExpiredCursor(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	token, err := codec.Encode(State{Route: "catalogs", Subject: "member-one", Limit: 20})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	codec.now = func() time.Time {
		return time.Date(2026, 7, 30, 12, 15, 0, 0, time.UTC)
	}
	_, err = codec.Decode(token, "catalogs", "member-one")
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Decode error = %v, want ErrExpired", err)
	}
}

func TestValidationRejectsUnsafeOrOversizedState(t *testing.T) {
	t.Parallel()

	codec := testCodec(t)
	tests := []State{
		{Route: "", Subject: "member", Limit: 20},
		{Route: "catalogs", Subject: "", Limit: 20},
		{Route: "catalogs", Subject: "member", Limit: 0},
		{Route: "catalogs", Subject: "member", Limit: 101},
		{
			Route:   "catalogs",
			Subject: "member",
			Limit:   20,
			Filters: map[string]string{"Bad-Key": "value"},
		},
		{
			Route:    "catalogs",
			Subject:  "member",
			Limit:    20,
			Position: []string{strings.Repeat("x", maxValueLength+1)},
		},
	}
	for _, input := range tests {
		if _, err := codec.Encode(input); err == nil {
			t.Fatalf("Encode(%#v) unexpectedly succeeded", input)
		}
	}
}

func TestNewValidatesKeyAndTTL(t *testing.T) {
	t.Parallel()

	if _, err := New([]byte("short"), 15*time.Minute); err == nil {
		t.Fatal("New accepted a short key")
	}
	if _, err := New(make([]byte, 32), time.Second); err == nil {
		t.Fatal("New accepted a short TTL")
	}
	if _, err := New(nil, 15*time.Minute); err != nil {
		t.Fatalf("New with generated key: %v", err)
	}
}
