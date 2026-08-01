package qualification

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository/sqliterepo"
)

const fixtureTimestamp = "2026-01-01T00:00:00Z"

type fixture struct {
	report                 FixtureReport
	userIDs                []string
	catalogIDs             []string
	itemIDs                []string
	crossUserIDs           []string
	sameUserIDs            []string
	oneWayUserID           string
	oneWayOtherID          string
	oneWayReceiveID        string
	unfulfillableUserID    string
	unfulfillableOtherID   string
	unfulfillableReceiveID string
	unfulfillableRequestID string
	writeTargetByID        map[string]string
}

func seedFixture(
	ctx context.Context,
	store *sqliterepo.Store,
	config Config,
) (fixture, error) {
	started := time.Now()
	digest := sha256.New()

	loaded, catalogIDs, itemIDs, err := generatedManifests(config, digest)
	if err != nil {
		return fixture{}, err
	}
	if err := store.ImportManifests(ctx, loaded); err != nil {
		return fixture{}, fmt.Errorf("import qualification catalogs: %w", err)
	}

	pairs, samePairCount, crossPairCount, err := reciprocalPairs(config, itemIDs)
	if err != nil {
		return fixture{}, err
	}
	oneWayPairIndex := selectReservedPair(
		pairs,
		samePairCount,
		crossPairCount,
		-1,
	)
	if oneWayPairIndex < 0 {
		return fixture{}, errors.New("qualification fixture cannot reserve a one-way pair")
	}
	if pairs[oneWayPairIndex].cross {
		crossPairCount--
	} else {
		samePairCount--
	}
	unfulfillablePairIndex := selectReservedPair(
		pairs,
		samePairCount,
		crossPairCount,
		oneWayPairIndex,
	)
	if unfulfillablePairIndex < 0 {
		return fixture{}, errors.New(
			"qualification fixture cannot reserve an unfulfillable pair",
		)
	}
	if pairs[unfulfillablePairIndex].cross {
		crossPairCount--
	} else {
		samePairCount--
	}
	database, err := openQualificationDatabase(config)
	if err != nil {
		return fixture{}, err
	}
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		return fixture{}, fmt.Errorf("ping qualification seed database: %w", err)
	}

	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fixture{}, fmt.Errorf("begin qualification seed: %w", err)
	}
	defer transaction.Rollback()
	userStatement, err := transaction.PrepareContext(
		ctx,
		`INSERT INTO users(id, display_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		return fixture{}, fmt.Errorf("prepare qualification users: %w", err)
	}
	defer userStatement.Close()
	entryStatement, err := transaction.PrepareContext(
		ctx,
		`INSERT INTO collection_entries(
			user_id, item_id, owned, available, available_since_at,
			wanted, wanted_since_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fixture{}, fmt.Errorf("prepare qualification entries: %w", err)
	}
	defer entryStatement.Close()

	result := fixture{
		userIDs:         make([]string, config.Users),
		catalogIDs:      catalogIDs,
		itemIDs:         itemIDs,
		writeTargetByID: make(map[string]string, config.Users),
	}
	rowsPerUser := config.CollectionRows / config.Users
	for pairIndex, pair := range pairs {
		leftUserIndex := pairIndex * 2
		rightUserIndex := leftUserIndex + 1
		leftUserID, err := insertSyntheticUser(
			ctx,
			userStatement,
			entryStatement,
			digest,
			leftUserIndex,
			pair.left,
			pair.right,
			rowsPerUser,
			itemIDs,
			result.writeTargetByID,
		)
		if err != nil {
			return fixture{}, err
		}
		rightWantItemIndex := pair.left
		switch pairIndex {
		case oneWayPairIndex:
			rightWantItemIndex = -1
		case unfulfillablePairIndex:
			rightWantItemIndex = unavailableRequestItemIndex(pair, len(itemIDs))
		}
		rightUserID, err := insertSyntheticUser(
			ctx,
			userStatement,
			entryStatement,
			digest,
			rightUserIndex,
			pair.right,
			rightWantItemIndex,
			rowsPerUser,
			itemIDs,
			result.writeTargetByID,
		)
		if err != nil {
			return fixture{}, err
		}
		result.userIDs[leftUserIndex] = leftUserID
		result.userIDs[rightUserIndex] = rightUserID
		if pairIndex == oneWayPairIndex {
			result.oneWayUserID = leftUserID
			result.oneWayOtherID = rightUserID
			result.oneWayReceiveID = itemIDs[pair.right]
			continue
		}
		if pairIndex == unfulfillablePairIndex {
			result.unfulfillableUserID = leftUserID
			result.unfulfillableOtherID = rightUserID
			result.unfulfillableReceiveID = itemIDs[pair.right]
			result.unfulfillableRequestID = itemIDs[rightWantItemIndex]
			continue
		}
		if pair.cross {
			result.crossUserIDs = append(result.crossUserIDs, leftUserID, rightUserID)
		} else {
			result.sameUserIDs = append(result.sameUserIDs, leftUserID, rightUserID)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fixture{}, fmt.Errorf("commit qualification seed: %w", err)
	}

	counts, err := readFixtureCounts(ctx, database)
	if err != nil {
		return fixture{}, err
	}
	seedDuration := time.Since(started)
	result.report = FixtureReport{
		SHA256:                   hex.EncodeToString(digest.Sum(nil)),
		SeedDuration:             seedDuration.String(),
		SeedDurationMilliseconds: durationMilliseconds(seedDuration),
		BrandCount:               counts.brands,
		CatalogCount:             counts.catalogs,
		ItemCount:                counts.items,
		UserCount:                counts.users,
		CollectionRowCount:       counts.entries,
		RowsPerUser:              rowsPerUser,
		CollectionDensityPercent: float64(counts.entries) * 100 /
			float64(counts.users*counts.items),
		SameCatalogReciprocalPairs:  samePairCount,
		CrossCatalogReciprocalPairs: crossPairCount,
	}
	if result.report.BrandCount != config.Brands ||
		result.report.CatalogCount != config.Catalogs ||
		result.report.ItemCount != config.Catalogs*config.ItemsPerCatalog ||
		result.report.UserCount != config.Users ||
		result.report.CollectionRowCount != config.CollectionRows {
		return fixture{}, fmt.Errorf(
			"qualification fixture counts differ from requested shape: %+v",
			result.report,
		)
	}
	return result, nil
}

func insertSyntheticUser(
	ctx context.Context,
	userStatement *sql.Stmt,
	entryStatement *sql.Stmt,
	digest hash.Hash,
	userIndex int,
	offerItemIndex int,
	wantItemIndex int,
	rowsPerUser int,
	itemIDs []string,
	writeTargetByID map[string]string,
) (string, error) {
	userID := fmt.Sprintf("synthetic-member-%05d", userIndex+1)
	displayName := fmt.Sprintf("Synthetic Member %05d", userIndex+1)
	user := domain.User{ID: userID, DisplayName: displayName}
	if err := user.Validate(); err != nil {
		return "", fmt.Errorf("validate generated qualification user: %w", err)
	}
	if _, err := userStatement.ExecContext(
		ctx,
		user.ID,
		user.DisplayName,
		fixtureTimestamp,
		fixtureTimestamp,
	); err != nil {
		return "", fmt.Errorf("insert qualification user %q: %w", userID, err)
	}
	writeDigestRecord(digest, "user", []byte(userID), []byte(displayName))

	type entry struct {
		itemIndex int
		owned     int
		available int
		wanted    int
	}
	entries := []entry{{itemIndex: offerItemIndex, owned: 2, available: 1}}
	used := map[int]struct{}{
		offerItemIndex: {},
	}
	if wantItemIndex >= 0 {
		entries = append(entries, entry{itemIndex: wantItemIndex, wanted: 1})
		used[wantItemIndex] = struct{}{}
	}
	for offset := 0; len(entries) < rowsPerUser; offset++ {
		candidate := (userIndex*7 + offset) % len(itemIDs)
		if _, duplicate := used[candidate]; duplicate {
			continue
		}
		used[candidate] = struct{}{}
		entries = append(entries, entry{
			itemIndex: candidate,
			owned:     1 + (userIndex+offset)%2,
		})
	}
	writeTargetByID[userID] = itemIDs[entries[2].itemIndex]
	for _, candidate := range entries {
		itemID := itemIDs[candidate.itemIndex]
		var availableSince, wantedSince any
		if candidate.available > 0 {
			availableSince = fixtureTimestamp
		}
		if candidate.wanted > 0 {
			wantedSince = fixtureTimestamp
		}
		if _, err := entryStatement.ExecContext(
			ctx,
			userID,
			itemID,
			candidate.owned,
			candidate.available,
			availableSince,
			candidate.wanted,
			wantedSince,
			fixtureTimestamp,
		); err != nil {
			return "", fmt.Errorf(
				"insert qualification collection row for %q/%q: %w",
				userID,
				itemID,
				err,
			)
		}
		writeDigestRecord(
			digest,
			"entry",
			[]byte(userID),
			[]byte(itemID),
			[]byte(fmt.Sprintf(
				"%d/%d/%d",
				candidate.owned,
				candidate.available,
				candidate.wanted,
			)),
		)
	}
	return userID, nil
}

func selectReservedPair(
	pairs []reciprocalPair,
	sameCount int,
	crossCount int,
	excludedIndex int,
) int {
	for index, pair := range pairs {
		if index == excludedIndex {
			continue
		}
		if pair.cross && crossCount > 1 {
			return index
		}
		if !pair.cross && sameCount > 1 {
			return index
		}
	}
	return -1
}

func unavailableRequestItemIndex(pair reciprocalPair, itemCount int) int {
	for index := range itemCount {
		if index != pair.left && index != pair.right {
			return index
		}
	}
	panic("qualification fixture needs at least three catalog items")
}

func writeDigestRecord(target hash.Hash, kind string, fields ...[]byte) {
	target.Write([]byte(kind))
	target.Write([]byte{0})
	for _, field := range fields {
		target.Write(field)
		target.Write([]byte{0})
	}
}

type fixtureCounts struct {
	brands   int
	catalogs int
	items    int
	users    int
	entries  int
}

func readFixtureCounts(ctx context.Context, database *sql.DB) (fixtureCounts, error) {
	var counts fixtureCounts
	err := database.QueryRowContext(
		ctx,
		`SELECT
			(SELECT count(*) FROM brands),
			(SELECT count(*) FROM catalogs),
			(SELECT count(*) FROM catalog_items),
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM collection_entries)`,
	).Scan(
		&counts.brands,
		&counts.catalogs,
		&counts.items,
		&counts.users,
		&counts.entries,
	)
	if err != nil {
		return fixtureCounts{}, fmt.Errorf("read qualification fixture counts: %w", err)
	}
	return counts, nil
}
