// Package service implements $OOSHDAQ use cases independently of transport
// and storage technology.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

// Dependencies keeps the service coupled to behavior-focused ports rather
// than one storage-shaped interface.
type Dependencies struct {
	Health      repository.HealthChecker
	Users       repository.UserStore
	Activity    repository.UserActivityWriter
	Catalogs    repository.CatalogReader
	Collections repository.CollectionReader
	Writes      repository.CollectionWriter
	Matches     repository.MatchFinder
}

// Service coordinates authenticated member use cases through narrow ports.
type Service struct {
	health      repository.HealthChecker
	users       repository.UserStore
	activity    repository.UserActivityWriter
	catalogs    repository.CatalogReader
	collections repository.CollectionReader
	writes      repository.CollectionWriter
	matches     repository.MatchFinder
}

// New constructs a service.
func New(dependencies Dependencies) (*Service, error) {
	switch {
	case dependencies.Health == nil:
		return nil, errors.New("service: health checker is required")
	case dependencies.Users == nil:
		return nil, errors.New("service: user store is required")
	case dependencies.Activity == nil:
		return nil, errors.New("service: user activity writer is required")
	case dependencies.Catalogs == nil:
		return nil, errors.New("service: catalog reader is required")
	case dependencies.Collections == nil:
		return nil, errors.New("service: collection reader is required")
	case dependencies.Writes == nil:
		return nil, errors.New("service: collection writer is required")
	case dependencies.Matches == nil:
		return nil, errors.New("service: match finder is required")
	}
	return &Service{
		health:      dependencies.Health,
		users:       dependencies.Users,
		activity:    dependencies.Activity,
		catalogs:    dependencies.Catalogs,
		collections: dependencies.Collections,
		writes:      dependencies.Writes,
		matches:     dependencies.Matches,
	}, nil
}

// Ready verifies that the backing repository is reachable.
func (s *Service) Ready(ctx context.Context) error {
	return s.health.Ping(ctx)
}

// User returns a persisted user profile.
func (s *Service) User(ctx context.Context, userID string) (domain.User, error) {
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// TouchUserActivity records the coarse authenticated-activity signal used by
// match ranking. The repository enforces the write-rate limit.
func (s *Service) TouchUserActivity(ctx context.Context, userID string) error {
	if err := s.activity.TouchUserLastSeen(ctx, userID); err != nil {
		return fmt.Errorf("touch user activity: %w", err)
	}
	return nil
}

// EnsureUser loads an existing user or creates a minimal local profile for a
// newly authenticated proxy identity. A nonempty display name refreshes an
// existing profile, allowing identity-provider changes to be reflected.
func (s *Service) EnsureUser(ctx context.Context, userID, displayName string) (domain.User, error) {
	existing, err := s.users.GetUser(ctx, userID)
	switch {
	case err == nil:
		if strings.TrimSpace(displayName) == "" || existing.DisplayName == strings.TrimSpace(displayName) {
			return existing, nil
		}
	case !errors.Is(err, repository.ErrNotFound):
		return domain.User{}, fmt.Errorf("get authenticated user: %w", err)
	}

	if strings.TrimSpace(displayName) == "" {
		displayName = "$OOSHDAQ user"
	}
	user := domain.User{ID: userID, DisplayName: strings.TrimSpace(displayName)}
	if err := user.Validate(); err != nil {
		return domain.User{}, err
	}
	if err := s.users.UpsertUser(ctx, user); err != nil {
		return domain.User{}, fmt.Errorf("save authenticated user: %w", err)
	}
	return user, nil
}

func (s *Service) Catalogs(
	ctx context.Context,
	query repository.CatalogQuery,
) (repository.CatalogPage, error) {
	page, err := s.catalogs.ListCatalogs(ctx, query)
	if err != nil {
		return repository.CatalogPage{}, fmt.Errorf("list catalogs: %w", err)
	}
	return page, nil
}

func (s *Service) Collection(
	ctx context.Context,
	query repository.CollectionQuery,
) (repository.CollectionPage, error) {
	page, err := s.collections.ListCollectionPage(ctx, query)
	if err != nil {
		return repository.CollectionPage{}, fmt.Errorf("list collection: %w", err)
	}
	return page, nil
}

func (s *Service) CollectionSummary(
	ctx context.Context,
	userID string,
) (repository.OverallCollectionSummary, error) {
	summary, err := s.collections.GetCollectionSummary(ctx, userID)
	if err != nil {
		return repository.OverallCollectionSummary{}, fmt.Errorf(
			"get collection summary: %w",
			err,
		)
	}
	return summary, nil
}

// UpdateCollection validates and persists one user's catalog quantity record.
func (s *Service) UpdateCollection(
	ctx context.Context,
	userID string,
	itemID string,
	owned int,
	available int,
	wanted int,
) (repository.CollectionRecord, error) {
	write := repository.CollectionWrite{
		UserID:    userID,
		ItemID:    itemID,
		Owned:     owned,
		Available: available,
		Wanted:    wanted,
	}
	if err := write.Validate(); err != nil {
		return repository.CollectionRecord{}, err
	}
	record, err := s.writes.SaveCollectionEntry(ctx, write)
	if err != nil {
		return repository.CollectionRecord{}, fmt.Errorf("save collection entry: %w", err)
	}
	return record, nil
}

func (s *Service) Matches(
	ctx context.Context,
	query repository.MatchQuery,
) (repository.MatchPage, error) {
	page, err := s.matches.FindMatches(ctx, query)
	if err != nil {
		return repository.MatchPage{}, fmt.Errorf("find matches: %w", err)
	}
	return page, nil
}
