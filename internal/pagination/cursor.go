// Package pagination creates short-lived, authenticated keyset cursors.
//
// Cursors carry query filters and an immutable ordering position. They do not
// provide a database snapshot: callers must still treat changing result sets as
// live suggestions.
package pagination

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	tokenVersion   = 1
	defaultTTL     = 15 * time.Minute
	maxTokenLength = 4096
	maxRouteLength = 200
	maxValueLength = 256
	maxFilters     = 12
	maxPosition    = 12
)

var (
	// ErrInvalid means a cursor is malformed, tampered with, or bound to a
	// different route or principal.
	ErrInvalid = errors.New("invalid cursor")
	// ErrExpired means a well-formed cursor is past its short continuation
	// window.
	ErrExpired = errors.New("expired cursor")
)

// State is the server-owned continuation state encoded in a cursor.
type State struct {
	Route    string            `json:"route"`
	Subject  string            `json:"subject"`
	Filters  map[string]string `json:"filters,omitempty"`
	Limit    int               `json:"limit"`
	Position []string          `json:"position,omitempty"`
}

type payload struct {
	Version   int               `json:"version"`
	Route     string            `json:"route"`
	Subject   string            `json:"subject"`
	Filters   map[string]string `json:"filters,omitempty"`
	Limit     int               `json:"limit"`
	Position  []string          `json:"position,omitempty"`
	ExpiresAt int64             `json:"expires_at"`
}

// Codec encrypts and authenticates cursor payloads with a process-local key.
type Codec struct {
	aead cipher.AEAD
	ttl  time.Duration
	now  func() time.Time
}

// New constructs a cursor codec. A missing key generates a new 256-bit key, so
// cursors intentionally become invalid when the process restarts.
func New(key []byte, ttl time.Duration) (*Codec, error) {
	if ttl == 0 {
		ttl = defaultTTL
	}
	if ttl < time.Minute || ttl > time.Hour {
		return nil, errors.New("pagination: TTL must be between one minute and one hour")
	}
	copiedKey := append([]byte(nil), key...)
	if len(copiedKey) == 0 {
		copiedKey = make([]byte, 32)
		if _, err := rand.Read(copiedKey); err != nil {
			return nil, fmt.Errorf("pagination: generate cursor key: %w", err)
		}
	}
	if len(copiedKey) < 32 {
		return nil, errors.New("pagination: cursor key must contain at least 32 bytes")
	}
	keyDigest := sha256.Sum256(copiedKey)
	block, err := aes.NewCipher(keyDigest[:])
	if err != nil {
		return nil, fmt.Errorf("pagination: create cursor cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("pagination: create cursor AEAD: %w", err)
	}
	return &Codec{aead: aead, ttl: ttl, now: time.Now}, nil
}

// Encode seals one continuation state into an opaque token.
func (c *Codec) Encode(state State) (string, error) {
	if c == nil {
		return "", errors.New("pagination: codec is nil")
	}
	if err := validateState(state); err != nil {
		return "", err
	}
	body, err := json.Marshal(payload{
		Version:   tokenVersion,
		Route:     state.Route,
		Subject:   state.Subject,
		Filters:   cloneMap(state.Filters),
		Limit:     state.Limit,
		Position:  append([]string(nil), state.Position...),
		ExpiresAt: c.now().Add(c.ttl).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("pagination: encode cursor: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("pagination: generate cursor nonce: %w", err)
	}
	sealed := c.aead.Seal(
		nil,
		nonce,
		body,
		cursorAssociatedData(state.Route, state.Subject),
	)
	token := base64.RawURLEncoding.EncodeToString(append(nonce, sealed...))
	if len(token) > maxTokenLength {
		return "", errors.New("pagination: encoded cursor is too large")
	}
	return token, nil
}

// Decode verifies a token and binds it to the expected route and principal.
// The returned filters and position are defensive copies.
func (c *Codec) Decode(token, expectedRoute, expectedSubject string) (State, error) {
	if c == nil || token == "" || len(token) > maxTokenLength {
		return State{}, ErrInvalid
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(sealed) <= c.aead.NonceSize() {
		return State{}, ErrInvalid
	}
	nonce := sealed[:c.aead.NonceSize()]
	ciphertext := sealed[c.aead.NonceSize():]
	body, err := c.aead.Open(
		nil,
		nonce,
		ciphertext,
		cursorAssociatedData(expectedRoute, expectedSubject),
	)
	if err != nil {
		return State{}, ErrInvalid
	}
	var decoded payload
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return State{}, ErrInvalid
	}
	if decoded.Version != tokenVersion ||
		decoded.Route != expectedRoute ||
		decoded.Subject != expectedSubject {
		return State{}, ErrInvalid
	}
	state := State{
		Route:    decoded.Route,
		Subject:  decoded.Subject,
		Filters:  cloneMap(decoded.Filters),
		Limit:    decoded.Limit,
		Position: append([]string(nil), decoded.Position...),
	}
	if err := validateState(state); err != nil {
		return State{}, ErrInvalid
	}
	if decoded.ExpiresAt <= 0 {
		return State{}, ErrInvalid
	}
	if !c.now().Before(time.Unix(decoded.ExpiresAt, 0)) {
		return State{}, ErrExpired
	}
	return state, nil
}

func cursorAssociatedData(route, subject string) []byte {
	return []byte(route + "\x00" + subject)
}

func validateState(state State) error {
	if !validValue(state.Route, maxRouteLength) {
		return errors.New("pagination: route is invalid")
	}
	if !validValue(state.Subject, maxValueLength) {
		return errors.New("pagination: subject is invalid")
	}
	if state.Limit < 1 || state.Limit > 100 {
		return errors.New("pagination: limit must be between 1 and 100")
	}
	if len(state.Filters) > maxFilters {
		return errors.New("pagination: too many filters")
	}
	for key, value := range state.Filters {
		if !validFilterKey(key) || !validOptionalValue(value) {
			return errors.New("pagination: filter is invalid")
		}
	}
	if len(state.Position) > maxPosition {
		return errors.New("pagination: position has too many values")
	}
	for _, value := range state.Position {
		if !validOptionalValue(value) {
			return errors.New("pagination: position is invalid")
		}
	}
	return nil
}

func validValue(value string, max int) bool {
	return value != "" && len(value) <= max && utf8.ValidString(value)
}

func validOptionalValue(value string) bool {
	return len(value) <= maxValueLength && utf8.ValidString(value)
}

func validFilterKey(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case character == '_':
		default:
			return false
		}
	}
	return true
}

func cloneMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
