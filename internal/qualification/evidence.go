package qualification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/repository"
	"github.com/PatrickQuintal/ooshdaq/internal/repository/sqliterepo"
)

func verifyPossibleSwapJourneys(
	ctx context.Context,
	store *sqliterepo.Store,
	fixture fixture,
) (bool, bool, bool, bool, error) {
	cross, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID: fixture.crossUserIDs[0],
		Scope:  repository.MatchScopeCrossCatalog,
		Limit:  repository.DefaultMatchPageSize,
	})
	if err != nil {
		return false, false, false, false, fmt.Errorf(
			"verify cross-catalog reciprocal journey: %w",
			err,
		)
	}
	crossPassed := false
	for _, match := range cross.Matches {
		if match.TheyRequest != nil &&
			match.TheyRequest.Fulfillable &&
			match.TheyRequest.CatalogItem.Catalog.ID !=
				match.YouReceive.CatalogItem.Catalog.ID {
			crossPassed = true
			break
		}
	}

	same, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID: fixture.sameUserIDs[0],
		Scope:  repository.MatchScopeSameCatalog,
		Limit:  repository.DefaultMatchPageSize,
	})
	if err != nil {
		return crossPassed, false, false, false, fmt.Errorf(
			"verify same-catalog reciprocal journey: %w",
			err,
		)
	}
	samePassed := false
	for _, match := range same.Matches {
		if match.TheyRequest != nil &&
			match.TheyRequest.Fulfillable &&
			match.TheyRequest.CatalogItem.Catalog.ID ==
				match.YouReceive.CatalogItem.Catalog.ID {
			samePassed = true
			break
		}
	}
	otherSummary, err := store.GetCollectionSummary(ctx, fixture.oneWayOtherID)
	if err != nil {
		return crossPassed, samePassed, false, false, fmt.Errorf(
			"verify one-way counterpart collection summary: %w",
			err,
		)
	}
	if otherSummary.MemberSummary.WantedDesigns != 0 {
		return crossPassed, samePassed, false, false, fmt.Errorf(
			"one-way counterpart %q unexpectedly has wanted designs %d",
			fixture.oneWayOtherID,
			otherSummary.MemberSummary.WantedDesigns,
		)
	}
	oneWay, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID: fixture.oneWayUserID,
		Limit:  repository.DefaultMatchPageSize,
	})
	if err != nil {
		return crossPassed, samePassed, false, false, fmt.Errorf(
			"verify unfiltered one-way possible-swap journey: %w",
			err,
		)
	}
	oneWayPassed := false
	for _, match := range oneWay.Matches {
		if match.OtherUser.ID == fixture.oneWayOtherID &&
			match.TheyRequest == nil &&
			match.YouReceive.CatalogItem.Item.ID == fixture.oneWayReceiveID {
			oneWayPassed = true
			break
		}
	}

	targetActionable, err := findJourneyMatch(
		ctx,
		store,
		repository.MatchQuery{
			UserID:    fixture.unfulfillableUserID,
			Readiness: repository.MatchReadinessActionable,
			Limit:     repository.MaxMatchPageSize,
		},
		func(match repository.MatchRecord) bool {
			return match.OtherUser.ID == fixture.unfulfillableOtherID &&
				match.YouReceive.CatalogItem.Item.ID ==
					fixture.unfulfillableReceiveID
		},
	)
	if err != nil {
		return crossPassed, samePassed, oneWayPassed, false, fmt.Errorf(
			"verify actionable side of split possible-swap journey: %w",
			err,
		)
	}
	if targetActionable {
		return crossPassed, samePassed, oneWayPassed, false, fmt.Errorf(
			"unfulfillable counterpart %q leaked into actionable feed",
			fixture.unfulfillableOtherID,
		)
	}
	targetUnfulfillable, err := findJourneyMatch(
		ctx,
		store,
		repository.MatchQuery{
			UserID:    fixture.unfulfillableUserID,
			Readiness: repository.MatchReadinessUnfulfillable,
			Limit:     repository.MaxMatchPageSize,
		},
		func(match repository.MatchRecord) bool {
			return match.OtherUser.ID == fixture.unfulfillableOtherID &&
				match.TheyRequest != nil &&
				!match.TheyRequest.Fulfillable &&
				match.TheyRequest.CatalogItem.Item.ID ==
					fixture.unfulfillableRequestID &&
				match.YouReceive.CatalogItem.Item.ID ==
					fixture.unfulfillableReceiveID
		},
	)
	if err != nil {
		return crossPassed, samePassed, oneWayPassed, false, fmt.Errorf(
			"verify unfulfillable possible-swap journey: %w",
			err,
		)
	}
	return crossPassed, samePassed, oneWayPassed, targetUnfulfillable, nil
}

func findJourneyMatch(
	ctx context.Context,
	store *sqliterepo.Store,
	query repository.MatchQuery,
	matches func(repository.MatchRecord) bool,
) (bool, error) {
	for {
		page, err := store.FindMatches(ctx, query)
		if err != nil {
			return false, err
		}
		for _, match := range page.Matches {
			if matches(match) {
				return true, nil
			}
		}
		if page.Next == nil {
			return false, nil
		}
		query.RankingAt = page.RankingAt
		query.After = page.Next
	}
}

func collectStorageAndPlan(
	ctx context.Context,
	store *sqliterepo.Store,
	config Config,
) (StorageReport, VerificationReport, error) {
	before, err := sqliteFileSizes(config.DatabasePath)
	if err != nil {
		return StorageReport{}, VerificationReport{}, err
	}
	database, err := openQualificationDatabase(config)
	if err != nil {
		return StorageReport{}, VerificationReport{}, err
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		return StorageReport{}, VerificationReport{}, fmt.Errorf(
			"ping qualification evidence database: %w",
			err,
		)
	}
	plan, indexes, bounded, err := possibleSwapQueryPlan(ctx, store)
	if err != nil {
		return StorageReport{}, VerificationReport{}, err
	}
	if err := database.Close(); err != nil {
		return StorageReport{}, VerificationReport{}, fmt.Errorf(
			"close qualification evidence database: %w",
			err,
		)
	}
	passiveResult, err := store.Checkpoint(ctx, false)
	if err != nil {
		return StorageReport{}, VerificationReport{}, err
	}
	truncateResult, err := store.Checkpoint(ctx, true)
	if err != nil {
		return StorageReport{}, VerificationReport{}, err
	}
	after, err := sqliteFileSizes(config.DatabasePath)
	if err != nil {
		return StorageReport{}, VerificationReport{}, err
	}
	return StorageReport{
			BeforeCheckpoint:   before,
			AfterCheckpoint:    after,
			PassiveCheckpoint:  checkpointReport(passiveResult),
			TruncateCheckpoint: checkpointReport(truncateResult),
		}, VerificationReport{
			QueryPlanBounded: bounded,
			QueryPlanDetails: plan,
			RequiredIndexes:  indexes,
		}, nil
}

func possibleSwapQueryPlan(
	ctx context.Context,
	store *sqliterepo.Store,
) ([]string, []string, bool, error) {
	var details []string
	rankingAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, feed := range []struct {
		name      string
		readiness repository.MatchReadiness
		scope     repository.MatchScope
	}{
		{
			name:      "actionable",
			readiness: repository.MatchReadinessActionable,
			scope:     repository.MatchScopeCrossCatalog,
		},
		{
			name:      "unfulfillable",
			readiness: repository.MatchReadinessUnfulfillable,
			scope:     repository.MatchScopeAll,
		},
	} {
		feedDetails, err := store.ExplainMatchQueryPlan(ctx, repository.MatchQuery{
			UserID:    "synthetic-member-00001",
			Readiness: feed.readiness,
			Scope:     feed.scope,
			RankingAt: rankingAt,
			Limit:     repository.DefaultMatchPageSize,
		})
		if err != nil {
			return nil, nil, false, fmt.Errorf(
				"explain qualification %s possible-swap query: %w",
				feed.name,
				err,
			)
		}
		for _, detail := range feedDetails {
			details = append(details, feed.name+": "+detail)
		}
	}

	requiredAliases := []string{
		"current_want",
		"other_offer",
		"candidate_offer",
		"candidate_want",
	}
	bounded := true
	for _, feedName := range []string{"actionable", "unfulfillable"} {
		for _, alias := range requiredAliases {
			foundSearch := false
			for _, detail := range details {
				if !strings.HasPrefix(detail, feedName+": ") {
					continue
				}
				if strings.Contains(detail, "SEARCH "+alias+" USING") {
					foundSearch = true
				}
				if strings.Contains(detail, "SCAN "+alias) {
					bounded = false
				}
			}
			if !foundSearch {
				bounded = false
			}
		}
	}
	indexes := queryPlanIndexes(details)
	hasOffers, hasWants := false, false
	for _, index := range indexes {
		if strings.Contains(index, "offers") {
			hasOffers = true
		}
		if strings.Contains(index, "wants") {
			hasWants = true
		}
	}
	bounded = bounded && hasOffers && hasWants
	return details, indexes, bounded, nil
}

func queryPlanIndexes(details []string) []string {
	unique := make(map[string]struct{})
	for _, detail := range details {
		fields := strings.Fields(detail)
		for index, field := range fields {
			if field == "INDEX" && index+1 < len(fields) {
				name := strings.Trim(fields[index+1], "()")
				unique[name] = struct{}{}
			}
			if field == "PRIMARY" && index+1 < len(fields) && fields[index+1] == "KEY" {
				unique["PRIMARY KEY"] = struct{}{}
			}
		}
	}
	indexes := make([]string, 0, len(unique))
	for index := range unique {
		indexes = append(indexes, index)
	}
	sort.Strings(indexes)
	return indexes
}

func checkpointReport(result sqliterepo.CheckpointResult) CheckpointResult {
	return CheckpointResult{
		Busy:                 result.Busy,
		LogFrames:            result.LogFrames,
		CheckpointedFrames:   result.CheckpointedFrames,
		Duration:             result.Duration.String(),
		DurationMilliseconds: durationMilliseconds(result.Duration),
	}
}

func sqliteFileSizes(databasePath string) (FileSizeReport, error) {
	databaseBytes, err := fileSize(databasePath)
	if err != nil {
		return FileSizeReport{}, err
	}
	walBytes, err := fileSize(databasePath + "-wal")
	if err != nil {
		return FileSizeReport{}, err
	}
	shmBytes, err := fileSize(databasePath + "-shm")
	if err != nil {
		return FileSizeReport{}, err
	}
	return FileSizeReport{
		DatabaseBytes: databaseBytes,
		WALBytes:      walBytes,
		SHMBytes:      shmBytes,
	}, nil
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("stat qualification file %q: %w", path, err)
	}
	return info.Size(), nil
}
