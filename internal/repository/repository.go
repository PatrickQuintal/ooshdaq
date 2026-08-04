// Package repository defines behavior-focused persistence ports for $OOSHDAQ.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
)

const (
	DefaultCatalogPageSize    = 20
	MaxCatalogPageSize        = 50
	DefaultCollectionPageSize = 40
	MaxCollectionPageSize     = 100
	DefaultMatchPageSize      = 20
	MaxMatchPageSize          = 50
	MaxSearchRunes            = 100
	MatchWaitCap              = 60 * 24 * time.Hour
	MatchStaleAfter           = 90 * 24 * time.Hour
	LastSeenTouchInterval     = time.Hour
)

// FailureKind is a stable classification for repository failures. Adapters
// should preserve context cancellation and deadlines instead of wrapping them
// in a Failure.
type FailureKind string

const (
	FailureUnknown         FailureKind = ""
	FailureInvalidArgument FailureKind = "invalid_argument"
	FailureNotFound        FailureKind = "not_found"
	FailureConflict        FailureKind = "conflict"
	FailureBusy            FailureKind = "busy"
	FailureUnavailable     FailureKind = "unavailable"
	FailureReadOnly        FailureKind = "read_only"
	FailureFull            FailureKind = "full"
	FailureCorrupt         FailureKind = "corrupt"
	FailureIncompatible    FailureKind = "incompatible"
)

var (
	ErrInvalidArgument = errors.New("repository: invalid argument")
	ErrNotFound        = errors.New("repository: not found")
	ErrConflict        = errors.New("repository: conflict")
	ErrBusy            = errors.New("repository: busy")
	ErrUnavailable     = errors.New("repository: unavailable")
	ErrReadOnly        = errors.New("repository: read only")
	ErrFull            = errors.New("repository: full")
	ErrCorrupt         = errors.New("repository: corrupt")
	ErrIncompatible    = errors.New("repository: incompatible")
)

// Failure adds an operation and stable class to an adapter-specific cause.
// Callers should normally use errors.Is or Classify rather than inspect it.
type Failure struct {
	Kind      FailureKind
	Operation string
	Err       error
}

func (failure *Failure) Error() string {
	if failure == nil {
		return "<nil>"
	}
	message := "repository"
	if failure.Operation != "" {
		message += " " + failure.Operation
	}
	if failure.Err != nil {
		return message + ": " + failure.Err.Error()
	}
	if sentinel := sentinelFor(failure.Kind); sentinel != nil {
		return message + ": " + sentinel.Error()
	}
	return message + ": unknown failure"
}

func (failure *Failure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Err
}

func (failure *Failure) Is(target error) bool {
	if failure == nil {
		return false
	}
	return target != nil && target == sentinelFor(failure.Kind)
}

// NewFailure classifies cause without discarding it. A nil cause is useful
// when the class itself completely describes the failure.
func NewFailure(kind FailureKind, operation string, cause error) error {
	return &Failure{Kind: kind, Operation: operation, Err: cause}
}

// Classify reports a stable repository class, or FailureUnknown for an
// unclassified error (including context cancellation and deadlines).
func Classify(err error) FailureKind {
	if err == nil {
		return FailureUnknown
	}
	var failure *Failure
	if errors.As(err, &failure) && failure.Kind != FailureUnknown {
		return failure.Kind
	}
	for _, candidate := range []FailureKind{
		FailureInvalidArgument,
		FailureNotFound,
		FailureConflict,
		FailureBusy,
		FailureUnavailable,
		FailureReadOnly,
		FailureFull,
		FailureCorrupt,
		FailureIncompatible,
	} {
		if errors.Is(err, sentinelFor(candidate)) {
			return candidate
		}
	}
	return FailureUnknown
}

func sentinelFor(kind FailureKind) error {
	switch kind {
	case FailureInvalidArgument:
		return ErrInvalidArgument
	case FailureNotFound:
		return ErrNotFound
	case FailureConflict:
		return ErrConflict
	case FailureBusy:
		return ErrBusy
	case FailureUnavailable:
		return ErrUnavailable
	case FailureReadOnly:
		return ErrReadOnly
	case FailureFull:
		return ErrFull
	case FailureCorrupt:
		return ErrCorrupt
	case FailureIncompatible:
		return ErrIncompatible
	default:
		return nil
	}
}

// HealthChecker is the readiness-facing storage port.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// UserStore persists only the profile fields required by the product. Identity
// provider claims and credentials do not cross this boundary.
type UserStore interface {
	GetUser(ctx context.Context, id string) (domain.User, error)
	UpsertUser(ctx context.Context, user domain.User) error
}

// UserActivityWriter persists only the coarse, rate-limited activity signal
// used to avoid prioritizing abandoned exchange state.
type UserActivityWriter interface {
	// TouchUserLastSeen records authenticated activity, while implementations
	// enforce LastSeenTouchInterval so request frequency cannot amplify writes.
	TouchUserLastSeen(ctx context.Context, id string) error
}

// PublicationFilter controls which published catalogs are returned. The zero
// value means active catalogs.
type PublicationFilter string

const (
	PublicationFilterActive  PublicationFilter = "active"
	PublicationFilterRetired PublicationFilter = "retired"
	PublicationFilterAll     PublicationFilter = "all"
)

// CollectionState controls the member-state predicate for a collection page.
// The zero value means all published items.
type CollectionState string

const (
	CollectionStateAll       CollectionState = "all"
	CollectionStateOwned     CollectionState = "owned"
	CollectionStateAvailable CollectionState = "available"
	CollectionStateWanted    CollectionState = "wanted"
)

// MatchScope controls whether reciprocal legs may cross catalog boundaries.
// The zero value means both same- and cross-catalog results.
type MatchScope string

const (
	MatchScopeAll          MatchScope = "all"
	MatchScopeSameCatalog  MatchScope = "same_catalog"
	MatchScopeCrossCatalog MatchScope = "cross_catalog"
)

// MatchReadiness selects one disjoint possible-swap feed.
type MatchReadiness string

const (
	MatchReadinessActionable    MatchReadiness = "actionable"
	MatchReadinessUnfulfillable MatchReadiness = "unfulfillable"
)

// MemberSummary contains distinct positive item counts shown to members.
type MemberSummary struct {
	AvailableDesigns int
	WantedDesigns    int
}

// CatalogRecord is the repository projection required to render navigation.
// Artwork is a deterministic representative item artwork; presentation code
// remains responsible for converting reviewed domain metadata to public URLs.
type CatalogRecord struct {
	Brand           domain.Brand
	Catalog         domain.Catalog
	Artwork         domain.Artwork
	ItemCount       int
	UniverseOptions []string
	MemberSummary   MemberSummary
}

// CatalogItemRecord keeps a full item self-contained across match catalogs.
type CatalogItemRecord struct {
	Brand   domain.Brand
	Catalog domain.Catalog
	Item    domain.CatalogItem
}

// CollectionRecord is a published item plus one member's current quantities.
type CollectionRecord struct {
	CatalogItem CatalogItemRecord
	Owned       int
	Available   int
	Wanted      int
}

type CatalogPosition struct {
	SortOrder int
	CatalogID string
}

type CollectionPosition struct {
	ItemNumber int
	ItemID     string
}

type MatchPosition struct {
	RequestClass  int
	CappedSince   int64
	Stale         bool
	RequestItemID string
	CounterpartID string
	ReceiveItemID string
}

type CatalogQuery struct {
	UserID            string
	BrandID           string
	Search            string
	PublicationStatus PublicationFilter
	Limit             int
	After             *CatalogPosition
}

type CatalogPage struct {
	Catalogs []CatalogRecord
	Limit    int
	Next     *CatalogPosition
}

// CatalogReader performs the bounded navigation query. It never returns draft
// brands, catalogs, or items.
type CatalogReader interface {
	ListCatalogs(ctx context.Context, query CatalogQuery) (CatalogPage, error)
}

type CollectionQuery struct {
	UserID    string
	CatalogID string
	Search    string
	Universe  string
	State     CollectionState
	Limit     int
	After     *CollectionPosition
}

type CollectionPage struct {
	Catalog CatalogRecord
	Entries []CollectionRecord
	Limit   int
	Next    *CollectionPosition
}

type OverallCollectionSummary struct {
	PossibleSwapCount int
	MemberSummary     MemberSummary
}

// CollectionWrite replaces one sparse member/item record atomically.
type CollectionWrite struct {
	UserID    string
	ItemID    string
	Owned     int
	Available int
	Wanted    int
}

func (write CollectionWrite) Validate() error {
	return domain.CollectionEntry{
		UserID:    write.UserID,
		ItemID:    write.ItemID,
		Owned:     write.Owned,
		Available: write.Available,
		Wanted:    write.Wanted,
	}.Validate()
}

type CollectionReader interface {
	ListCollectionPage(ctx context.Context, query CollectionQuery) (CollectionPage, error)
	GetCollectionSummary(ctx context.Context, userID string) (OverallCollectionSummary, error)
}

type CollectionWriter interface {
	SaveCollectionEntry(ctx context.Context, write CollectionWrite) (CollectionRecord, error)
}

type MatchQuery struct {
	UserID           string
	GiveCatalogID    string
	ReceiveCatalogID string
	Scope            MatchScope
	Readiness        MatchReadiness
	RankingAt        time.Time
	Limit            int
	After            *MatchPosition
}

type MatchLeg struct {
	CatalogItem CatalogItemRecord
	Quantity    int
}

// MatchRequest identifies the one published item the counterpart would like.
// Quantity is the amount the current member can offer now, not the
// counterpart's private wanted quantity.
type MatchRequest struct {
	CatalogItem CatalogItemRecord
	Quantity    int
	Fulfillable bool
}

type MatchRecord struct {
	ID          string
	OtherUser   domain.User
	TheyRequest *MatchRequest
	YouReceive  MatchLeg
}

type MatchPage struct {
	Matches   []MatchRecord
	Limit     int
	RankingAt time.Time
	Next      *MatchPosition
}

// MatchFinder performs the bounded possible-swap query; implementations must
// not expose unrelated counterpart collection state.
type MatchFinder interface {
	FindMatches(ctx context.Context, query MatchQuery) (MatchPage, error)
}

func invalidArgument(operation, field, reason string) error {
	return NewFailure(
		FailureInvalidArgument,
		operation,
		domain.ValidationError{Field: field, Reason: reason},
	)
}

func notFound(operation, resource, id string) error {
	return NewFailure(
		FailureNotFound,
		operation,
		fmt.Errorf("%s %q does not exist", resource, id),
	)
}
