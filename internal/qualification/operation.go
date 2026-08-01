package qualification

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/ooshdaq/ooshdaq/internal/repository"
	"github.com/ooshdaq/ooshdaq/internal/repository/sqliterepo"
)

var errResponseLimit = errors.New("qualification response exceeded its hard limit")

func executeOperation(
	ctx context.Context,
	store *sqliterepo.Store,
	fixture fixture,
	config Config,
	workerIndex int,
	writeCount int,
	random *rand.Rand,
	operation string,
) error {
	switch operation {
	case operationCatalog:
		limit := min(config.Catalogs, repository.MaxCatalogPageSize)
		page, err := store.ListCatalogs(ctx, repository.CatalogQuery{
			UserID: fixture.userIDs[random.Intn(len(fixture.userIDs))],
			Limit:  limit,
		})
		if err != nil {
			return err
		}
		if len(page.Catalogs) > limit {
			return fmt.Errorf(
				"%w: catalog response had %d, limit %d",
				errResponseLimit,
				len(page.Catalogs),
				limit,
			)
		}
		return nil
	case operationCollection:
		limit := min(config.ItemsPerCatalog, repository.MaxCollectionPageSize)
		page, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
			UserID:    fixture.userIDs[random.Intn(len(fixture.userIDs))],
			CatalogID: fixture.catalogIDs[random.Intn(len(fixture.catalogIDs))],
			Limit:     limit,
		})
		if err != nil {
			return err
		}
		if len(page.Entries) > limit {
			return fmt.Errorf(
				"%w: collection response had %d, limit %d",
				errResponseLimit,
				len(page.Entries),
				limit,
			)
		}
		return nil
	case operationMatchesActionable:
		page, err := store.FindMatches(ctx, repository.MatchQuery{
			UserID:    fixture.crossUserIDs[random.Intn(len(fixture.crossUserIDs))],
			Scope:     repository.MatchScopeCrossCatalog,
			Readiness: repository.MatchReadinessActionable,
			Limit:     repository.DefaultMatchPageSize,
		})
		if err != nil {
			return err
		}
		if len(page.Matches) > repository.DefaultMatchPageSize {
			return fmt.Errorf(
				"%w: match response had %d, limit %d",
				errResponseLimit,
				len(page.Matches),
				repository.DefaultMatchPageSize,
			)
		}
		for _, match := range page.Matches {
			if match.TheyRequest == nil ||
				!match.TheyRequest.Fulfillable ||
				match.TheyRequest.CatalogItem.Catalog.ID ==
					match.YouReceive.CatalogItem.Catalog.ID {
				return errors.New("cross-catalog match response contained a same-catalog result")
			}
		}
		return nil
	case operationMatchesUnfulfillable:
		page, err := store.FindMatches(ctx, repository.MatchQuery{
			UserID:    fixture.unfulfillableUserID,
			Readiness: repository.MatchReadinessUnfulfillable,
			Limit:     repository.DefaultMatchPageSize,
		})
		if err != nil {
			return err
		}
		if len(page.Matches) > repository.DefaultMatchPageSize {
			return fmt.Errorf(
				"%w: unfulfillable match response had %d, limit %d",
				errResponseLimit,
				len(page.Matches),
				repository.DefaultMatchPageSize,
			)
		}
		if len(page.Matches) == 0 {
			return errors.New("unfulfillable match response was empty")
		}
		for _, match := range page.Matches {
			if match.TheyRequest == nil || match.TheyRequest.Fulfillable {
				return errors.New(
					"unfulfillable match response contained an actionable result",
				)
			}
		}
		return nil
	case operationWrite:
		userID := fixture.userIDs[workerIndex%len(fixture.userIDs)]
		owned := 1 + writeCount%2
		_, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
			UserID: userID,
			ItemID: fixture.writeTargetByID[userID],
			Owned:  owned,
		})
		return err
	default:
		return fmt.Errorf("unknown qualification operation %q", operation)
	}
}
