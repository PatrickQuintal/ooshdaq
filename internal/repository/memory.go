package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ooshdaq/ooshdaq/internal/domain"
)

type importedManifest struct {
	manifest domain.CatalogManifest
	digest   string
}

type collectionTiming struct {
	availableSince time.Time
	wantedSince    time.Time
}

// MemoryStore is the reference implementation of the behavior-focused ports.
// It is safe for concurrent use and is intended for conformance tests and
// ephemeral development, not capacity measurements.
type MemoryStore struct {
	mu sync.RWMutex

	brands    map[string]domain.Brand
	catalogs  map[string]domain.Catalog
	items     map[string]domain.CatalogItem
	manifests map[string]importedManifest
	users     map[string]domain.User
	entries   map[string]map[string]domain.CollectionEntry
	timings   map[string]map[string]collectionTiming
	offers    map[string]map[string]domain.CollectionEntry
	wants     map[string]map[string]domain.CollectionEntry
	lastSeen  map[string]time.Time
	now       func() time.Time
}

// NewMemoryStore returns an empty in-memory store. Catalog data enters through
// ImportManifest and member profiles through UpsertUser.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		brands:    make(map[string]domain.Brand),
		catalogs:  make(map[string]domain.Catalog),
		items:     make(map[string]domain.CatalogItem),
		manifests: make(map[string]importedManifest),
		users:     make(map[string]domain.User),
		entries:   make(map[string]map[string]domain.CollectionEntry),
		timings:   make(map[string]map[string]collectionTiming),
		offers:    make(map[string]map[string]domain.CollectionEntry),
		wants:     make(map[string]map[string]domain.CollectionEntry),
		lastSeen:  make(map[string]time.Time),
		now:       time.Now,
	}
}

func (store *MemoryStore) Ping(ctx context.Context) error {
	return contextError(ctx)
}

func (store *MemoryStore) GetUser(ctx context.Context, id string) (domain.User, error) {
	if err := contextError(ctx); err != nil {
		return domain.User{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	if err := contextError(ctx); err != nil {
		return domain.User{}, err
	}
	user, ok := store.users[id]
	if !ok {
		return domain.User{}, notFound("get user", "user", id)
	}
	return user, nil
}

func (store *MemoryStore) UpsertUser(ctx context.Context, user domain.User) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := user.Validate(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	if err := contextError(ctx); err != nil {
		return err
	}
	store.users[user.ID] = user
	return nil
}

func (store *MemoryStore) TouchUserLastSeen(ctx context.Context, id string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}
	if _, exists := store.users[id]; !exists {
		return notFound("touch user last seen", "user", id)
	}
	now := store.now().UTC()
	if seen := store.lastSeen[id]; !seen.IsZero() &&
		now.Before(seen.Add(LastSeenTouchInterval)) {
		return nil
	}
	store.lastSeen[id] = now
	return nil
}

func (store *MemoryStore) ImportManifest(
	ctx context.Context,
	input ManifestImport,
) (ManifestImportResult, error) {
	const operation = "import manifest"

	if err := contextError(ctx); err != nil {
		return ManifestImportResult{}, err
	}
	if err := input.Manifest.Validate(); err != nil {
		return ManifestImportResult{}, NewFailure(FailureInvalidArgument, operation, err)
	}
	if !validDigest(input.SHA256) {
		return ManifestImportResult{}, invalidArgument(
			operation,
			"sha256",
			"must be 64 lowercase hexadecimal characters",
		)
	}
	manifest := cloneManifest(input.Manifest)
	result := ManifestImportResult{
		CatalogID:       manifest.Catalog.ID,
		ManifestVersion: manifest.ManifestVersion,
		SHA256:          input.SHA256,
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return ManifestImportResult{}, err
	}

	previous, updating := store.manifests[manifest.Catalog.ID]
	if updating &&
		previous.manifest.ManifestVersion == manifest.ManifestVersion &&
		previous.digest == input.SHA256 {
		return result, nil
	}
	if updating && previous.manifest.ManifestVersion == manifest.ManifestVersion {
		return ManifestImportResult{}, NewFailure(
			FailureConflict,
			operation,
			fmt.Errorf(
				"catalog %q version %d already has a different digest",
				manifest.Catalog.ID,
				manifest.ManifestVersion,
			),
		)
	}
	if updating {
		if err := manifest.ValidateSuccessor(previous.manifest); err != nil {
			return ManifestImportResult{}, NewFailure(FailureConflict, operation, err)
		}
		if previous.manifest.Catalog.Status != domain.PublicationDraft &&
			manifest.Catalog.SortOrder != previous.manifest.Catalog.SortOrder {
			return ManifestImportResult{}, NewFailure(
				FailureConflict,
				operation,
				fmt.Errorf("published catalog %q sort order is immutable", manifest.Catalog.ID),
			)
		}
		previousItems := make(map[string]domain.CatalogItem, len(previous.manifest.Items))
		for _, item := range previous.manifest.Items {
			previousItems[item.ID] = item
		}
		for _, item := range manifest.Items {
			oldItem, exists := previousItems[item.ID]
			if exists && oldItem.Status != domain.PublicationDraft && item.Number != oldItem.Number {
				return ManifestImportResult{}, NewFailure(
					FailureConflict,
					operation,
					fmt.Errorf("published item %q number is immutable", item.ID),
				)
			}
		}
	} else if _, exists := store.catalogs[manifest.Catalog.ID]; exists {
		return ManifestImportResult{}, NewFailure(
			FailureConflict,
			operation,
			fmt.Errorf("catalog id %q is already in use", manifest.Catalog.ID),
		)
	}

	if existingBrand, exists := store.brands[manifest.Brand.ID]; exists &&
		existingBrand != manifest.Brand &&
		store.brandUsedByOtherCatalogLocked(manifest.Brand.ID, manifest.Catalog.ID) {
		return ManifestImportResult{}, NewFailure(
			FailureConflict,
			operation,
			fmt.Errorf("brand %q conflicts with another catalog", manifest.Brand.ID),
		)
	}
	for _, item := range manifest.Items {
		if existing, exists := store.items[item.ID]; exists &&
			existing.CatalogID != manifest.Catalog.ID {
			return ManifestImportResult{}, NewFailure(
				FailureConflict,
				operation,
				fmt.Errorf(
					"item id %q is already used by catalog %q",
					item.ID,
					existing.CatalogID,
				),
			)
		}
	}

	store.brands[manifest.Brand.ID] = manifest.Brand
	store.catalogs[manifest.Catalog.ID] = manifest.Catalog
	for _, item := range manifest.Items {
		store.items[item.ID] = cloneCatalogItem(item)
	}
	store.manifests[manifest.Catalog.ID] = importedManifest{
		manifest: manifest,
		digest:   input.SHA256,
	}
	result.Applied = true
	return result, nil
}

func (store *MemoryStore) ListCatalogs(
	ctx context.Context,
	query CatalogQuery,
) (CatalogPage, error) {
	const operation = "list catalogs"

	normalized, err := normalizeCatalogQuery(query)
	if err != nil {
		return CatalogPage{}, err
	}
	if err := contextError(ctx); err != nil {
		return CatalogPage{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return CatalogPage{}, err
	}
	if _, exists := store.users[normalized.UserID]; !exists {
		return CatalogPage{}, notFound(operation, "user", normalized.UserID)
	}

	candidates := make([]domain.Catalog, 0, len(store.catalogs))
	for _, catalog := range store.catalogs {
		if err := contextError(ctx); err != nil {
			return CatalogPage{}, err
		}
		brand, exists := store.brands[catalog.BrandID]
		if !exists || brand.Status == domain.PublicationDraft ||
			catalog.Status == domain.PublicationDraft {
			continue
		}
		if normalized.BrandID != "" && catalog.BrandID != normalized.BrandID {
			continue
		}
		if !catalogStatusMatches(catalog.Status, normalized.PublicationStatus) {
			continue
		}
		if normalized.Search != "" {
			matchesSearch := containsFold(brand.Name, normalized.Search) ||
				containsFold(catalog.Name, normalized.Search) ||
				containsFold(catalog.Edition, normalized.Search)
			if !matchesSearch {
				var searchErr error
				matchesSearch, searchErr = store.catalogItemsContainFoldLocked(
					ctx,
					catalog.ID,
					normalized.Search,
				)
				if searchErr != nil {
					return CatalogPage{}, searchErr
				}
			}
			if !matchesSearch {
				continue
			}
		}
		if normalized.After != nil &&
			!catalogAfter(catalog, *normalized.After) {
			continue
		}
		candidates = append(candidates, catalog)
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].SortOrder != candidates[right].SortOrder {
			return candidates[left].SortOrder < candidates[right].SortOrder
		}
		return candidates[left].ID < candidates[right].ID
	})

	more := len(candidates) > normalized.Limit
	if more {
		candidates = candidates[:normalized.Limit]
	}
	page := CatalogPage{
		Catalogs: make([]CatalogRecord, 0, len(candidates)),
		Limit:    normalized.Limit,
	}
	for _, catalog := range candidates {
		record, recordErr := store.catalogRecordLocked(ctx, normalized.UserID, catalog)
		if recordErr != nil {
			return CatalogPage{}, recordErr
		}
		page.Catalogs = append(page.Catalogs, record)
	}
	if more && len(candidates) != 0 {
		last := candidates[len(candidates)-1]
		page.Next = &CatalogPosition{SortOrder: last.SortOrder, CatalogID: last.ID}
	}
	return page, nil
}

func (store *MemoryStore) catalogItemsContainFoldLocked(
	ctx context.Context,
	catalogID string,
	normalizedSearch string,
) (bool, error) {
	imported, exists := store.manifests[catalogID]
	if !exists {
		return false, nil
	}
	for _, item := range imported.manifest.Items {
		if err := contextError(ctx); err != nil {
			return false, err
		}
		if item.Status == domain.PublicationDraft {
			continue
		}
		if containsFold(item.Name, normalizedSearch) ||
			containsFold(item.Universe, normalizedSearch) ||
			containsFold(item.Variant, normalizedSearch) {
			return true, nil
		}
	}
	return false, nil
}

func (store *MemoryStore) ListCollectionPage(
	ctx context.Context,
	query CollectionQuery,
) (CollectionPage, error) {
	const operation = "list collection page"

	normalized, err := normalizeCollectionQuery(query)
	if err != nil {
		return CollectionPage{}, err
	}
	if err := contextError(ctx); err != nil {
		return CollectionPage{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return CollectionPage{}, err
	}
	if _, exists := store.users[normalized.UserID]; !exists {
		return CollectionPage{}, notFound(operation, "user", normalized.UserID)
	}
	catalog, exists := store.publishedCatalogLocked(normalized.CatalogID)
	if !exists {
		return CollectionPage{}, notFound(operation, "catalog", normalized.CatalogID)
	}
	catalogRecord, err := store.catalogRecordLocked(ctx, normalized.UserID, catalog)
	if err != nil {
		return CollectionPage{}, err
	}
	if normalized.Universe != "" &&
		!sortedStringsContain(catalogRecord.UniverseOptions, normalized.Universe) {
		return CollectionPage{}, invalidArgument(
			operation,
			"universe",
			"is not a reviewed value for this catalog",
		)
	}

	entries := make([]CollectionRecord, 0)
	for _, item := range store.items {
		if err := contextError(ctx); err != nil {
			return CollectionPage{}, err
		}
		if item.CatalogID != catalog.ID || item.Status == domain.PublicationDraft {
			continue
		}
		if normalized.Search != "" &&
			!containsFold(item.Name, normalized.Search) &&
			!containsFold(item.Universe, normalized.Search) &&
			!containsFold(item.Variant, normalized.Search) {
			continue
		}
		if normalized.Universe != "" && item.Universe != normalized.Universe {
			continue
		}
		entry := store.entries[normalized.UserID][item.ID]
		if !collectionStateMatches(entry, normalized.State) {
			continue
		}
		if normalized.After != nil && !itemAfter(item, *normalized.After) {
			continue
		}
		entries = append(entries, store.collectionRecordLocked(item, entry))
	}
	sort.Slice(entries, func(left, right int) bool {
		leftItem := entries[left].CatalogItem.Item
		rightItem := entries[right].CatalogItem.Item
		if leftItem.Number != rightItem.Number {
			return leftItem.Number < rightItem.Number
		}
		return leftItem.ID < rightItem.ID
	})

	more := len(entries) > normalized.Limit
	if more {
		entries = entries[:normalized.Limit]
	}
	page := CollectionPage{
		Catalog: catalogRecord,
		Entries: entries,
		Limit:   normalized.Limit,
	}
	if more && len(entries) != 0 {
		last := entries[len(entries)-1].CatalogItem.Item
		page.Next = &CollectionPosition{ItemNumber: last.Number, ItemID: last.ID}
	}
	return page, nil
}

func (store *MemoryStore) GetCollectionSummary(
	ctx context.Context,
	userID string,
) (OverallCollectionSummary, error) {
	const operation = "get collection summary"

	if userID == "" {
		return OverallCollectionSummary{}, invalidArgument(operation, "user_id", "is required")
	}
	if err := contextError(ctx); err != nil {
		return OverallCollectionSummary{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return OverallCollectionSummary{}, err
	}
	if _, exists := store.users[userID]; !exists {
		return OverallCollectionSummary{}, notFound(operation, "user", userID)
	}

	var summary OverallCollectionSummary
	for _, catalog := range store.catalogs {
		if err := contextError(ctx); err != nil {
			return OverallCollectionSummary{}, err
		}
		brand, exists := store.brands[catalog.BrandID]
		if exists && brand.Status == domain.PublicationActive &&
			catalog.Status == domain.PublicationActive {
			summary.ActiveCatalogCount++
		}
	}
	for _, item := range store.items {
		if err := contextError(ctx); err != nil {
			return OverallCollectionSummary{}, err
		}
		catalog, catalogExists := store.catalogs[item.CatalogID]
		brand, brandExists := store.brands[catalog.BrandID]
		if item.Status == domain.PublicationActive &&
			catalogExists && catalog.Status == domain.PublicationActive &&
			brandExists && brand.Status == domain.PublicationActive {
			summary.ActiveItemCount++
		}
	}

	started := make(map[string]struct{})
	for itemID, entry := range store.entries[userID] {
		if err := contextError(ctx); err != nil {
			return OverallCollectionSummary{}, err
		}
		item, exists := store.items[itemID]
		if !exists || item.Status == domain.PublicationDraft {
			continue
		}
		if _, exists := store.publishedCatalogLocked(item.CatalogID); !exists {
			continue
		}
		addEntryToSummary(&summary.MemberSummary, entry)
		if entry.Owned > 0 || entry.Available > 0 || entry.Wanted > 0 {
			started[item.CatalogID] = struct{}{}
		}
		if entry.Wanted > 0 {
			for counterpartID := range store.offers[itemID] {
				if err := contextError(ctx); err != nil {
					return OverallCollectionSummary{}, err
				}
				if counterpartID != userID {
					summary.PossibleSwapCount++
				}
			}
		}
	}
	summary.CatalogsStarted = len(started)
	return summary, nil
}

func (store *MemoryStore) SaveCollectionEntry(
	ctx context.Context,
	write CollectionWrite,
) (CollectionRecord, error) {
	const operation = "save collection entry"

	if err := contextError(ctx); err != nil {
		return CollectionRecord{}, err
	}
	if err := write.Validate(); err != nil {
		return CollectionRecord{}, NewFailure(FailureInvalidArgument, operation, err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return CollectionRecord{}, err
	}
	if _, exists := store.users[write.UserID]; !exists {
		return CollectionRecord{}, notFound(operation, "user", write.UserID)
	}
	item, exists := store.publishedItemLocked(write.ItemID)
	if !exists {
		return CollectionRecord{}, notFound(operation, "item", write.ItemID)
	}

	previous := store.entries[write.UserID][write.ItemID]
	timing := store.timings[write.UserID][write.ItemID]
	now := store.now().UTC()
	timing.availableSince = transitionSince(
		previous.Available,
		write.Available,
		timing.availableSince,
		now,
	)
	timing.wantedSince = transitionSince(
		previous.Wanted,
		write.Wanted,
		timing.wantedSince,
		now,
	)
	entry := domain.CollectionEntry{
		UserID:    write.UserID,
		ItemID:    write.ItemID,
		Owned:     write.Owned,
		Available: write.Available,
		Wanted:    write.Wanted,
	}
	store.removeEntryIndexesLocked(store.entries[write.UserID][write.ItemID])
	if write.Owned == 0 && write.Available == 0 && write.Wanted == 0 {
		delete(store.entries[write.UserID], write.ItemID)
		delete(store.timings[write.UserID], write.ItemID)
		if len(store.entries[write.UserID]) == 0 {
			delete(store.entries, write.UserID)
		}
		if len(store.timings[write.UserID]) == 0 {
			delete(store.timings, write.UserID)
		}
	} else {
		if store.entries[write.UserID] == nil {
			store.entries[write.UserID] = make(map[string]domain.CollectionEntry)
		}
		if store.timings[write.UserID] == nil {
			store.timings[write.UserID] = make(map[string]collectionTiming)
		}
		store.entries[write.UserID][write.ItemID] = entry
		store.timings[write.UserID][write.ItemID] = timing
		store.addEntryIndexesLocked(entry)
	}
	return store.collectionRecordLocked(item, entry), nil
}

func transitionSince(
	previousQuantity int,
	nextQuantity int,
	since time.Time,
	now time.Time,
) time.Time {
	switch {
	case nextQuantity <= 0:
		return time.Time{}
	case previousQuantity <= 0 || since.IsZero():
		return now
	default:
		return since
	}
}

func (store *MemoryStore) FindMatches(
	ctx context.Context,
	query MatchQuery,
) (MatchPage, error) {
	const operation = "find matches"

	normalized, err := normalizeMatchQuery(query)
	if err != nil {
		return MatchPage{}, err
	}
	if err := contextError(ctx); err != nil {
		return MatchPage{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return MatchPage{}, err
	}
	if _, exists := store.users[normalized.UserID]; !exists {
		return MatchPage{}, notFound(operation, "user", normalized.UserID)
	}
	if normalized.RankingAt.IsZero() {
		normalized.RankingAt = store.now().UTC().Truncate(time.Second)
	} else {
		normalized.RankingAt = normalized.RankingAt.UTC().Truncate(time.Second)
	}
	if normalized.GiveCatalogID != "" {
		if _, exists := store.publishedCatalogLocked(normalized.GiveCatalogID); !exists {
			return emptyMatchPage(normalized.Limit), nil
		}
	}
	if normalized.ReceiveCatalogID != "" {
		if _, exists := store.publishedCatalogLocked(normalized.ReceiveCatalogID); !exists {
			return emptyMatchPage(normalized.Limit), nil
		}
	}

	currentEntries := store.entries[normalized.UserID]
	availableIDs := make([]string, 0)
	receiveIDs := make([]string, 0)
	for itemID, entry := range currentEntries {
		item, exists := store.publishedItemLocked(itemID)
		if !exists {
			continue
		}
		if entry.Available > 0 &&
			(normalized.GiveCatalogID == "" || item.CatalogID == normalized.GiveCatalogID) {
			availableIDs = append(availableIDs, itemID)
		}
		if entry.Wanted > 0 &&
			(normalized.ReceiveCatalogID == "" || item.CatalogID == normalized.ReceiveCatalogID) {
			receiveIDs = append(receiveIDs, itemID)
		}
	}
	sort.Strings(availableIDs)
	sort.Strings(receiveIDs)

	type candidate struct {
		position MatchPosition
		record   MatchRecord
	}
	candidates := make([]candidate, 0)
	requiresFulfillable := normalized.GiveCatalogID != "" ||
		normalized.Scope != MatchScopeAll
	for _, receiveID := range receiveIDs {
		for counterpartID := range store.offers[receiveID] {
			if err := contextError(ctx); err != nil {
				return MatchPage{}, err
			}
			if counterpartID == normalized.UserID {
				continue
			}
			if _, exists := store.users[counterpartID]; !exists {
				continue
			}
			receiveItem := store.items[receiveID]
			theirOffer := store.offers[receiveID][counterpartID]
			receiveQuantity := minimum(
				currentEntries[receiveID].Wanted,
				theirOffer.Available,
			)
			if receiveQuantity <= 0 {
				continue
			}
			fulfillable := store.preferredFulfillableRequestLocked(
				availableIDs,
				currentEntries,
				counterpartID,
				receiveItem,
				normalized.Scope,
				normalized.RankingAt,
			)
			var request *rankedMatchRequest
			requestClass := 0
			switch normalized.Readiness {
			case MatchReadinessActionable:
				if fulfillable != nil {
					request = fulfillable
				} else {
					if requiresFulfillable {
						continue
					}
					if store.preferredUnfulfillableRequestLocked(
						currentEntries,
						counterpartID,
						receiveItem,
						normalized.RankingAt,
					) != nil {
						continue
					}
					requestClass = 1
				}
			case MatchReadinessUnfulfillable:
				if fulfillable != nil {
					continue
				}
				request = store.preferredUnfulfillableRequestLocked(
					currentEntries,
					counterpartID,
					receiveItem,
					normalized.RankingAt,
				)
				if request == nil {
					continue
				}
			}

			position := MatchPosition{
				RequestClass:  requestClass,
				CounterpartID: counterpartID,
				ReceiveItemID: receiveID,
			}
			var projectedRequest *MatchRequest
			if request != nil {
				projected := request.request
				projectedRequest = &projected
				position.CappedSince = request.cappedSince
				position.Stale = request.stale
				position.RequestItemID = request.request.CatalogItem.Item.ID
			} else {
				position.CappedSince, position.Stale = matchWaitRank(
					store.timings[counterpartID][receiveID].availableSince,
					store.lastSeen[counterpartID],
					normalized.RankingAt,
				)
			}
			if normalized.After != nil && !matchAfter(position, *normalized.After) {
				continue
			}
			candidates = append(candidates, candidate{
				position: position,
				record: MatchRecord{
					ID:          matchID(normalized.UserID, counterpartID, receiveID),
					OtherUser:   store.users[counterpartID],
					TheyRequest: projectedRequest,
					YouReceive: MatchLeg{
						CatalogItem: store.catalogItemRecordLocked(receiveItem),
						Quantity:    receiveQuantity,
					},
				},
			})
		}
	}

	sort.Slice(candidates, func(left, right int) bool {
		return matchPositionLess(candidates[left].position, candidates[right].position)
	})
	more := len(candidates) > normalized.Limit
	if more {
		candidates = candidates[:normalized.Limit]
	}
	matches := make([]MatchRecord, 0, len(candidates))
	for _, candidate := range candidates {
		matches = append(matches, candidate.record)
	}
	page := MatchPage{
		Matches:   matches,
		Limit:     normalized.Limit,
		RankingAt: normalized.RankingAt,
	}
	if more && len(candidates) != 0 {
		next := candidates[len(candidates)-1].position
		page.Next = &next
	}
	return page, nil
}

func (store *MemoryStore) catalogRecordLocked(
	ctx context.Context,
	userID string,
	catalog domain.Catalog,
) (CatalogRecord, error) {
	record := CatalogRecord{
		Brand:           store.brands[catalog.BrandID],
		Catalog:         catalog,
		UniverseOptions: make([]string, 0),
	}
	universes := make(map[string]struct{})
	var representative *domain.CatalogItem
	for _, item := range store.items {
		if err := contextError(ctx); err != nil {
			return CatalogRecord{}, err
		}
		if item.CatalogID != catalog.ID || item.Status == domain.PublicationDraft {
			continue
		}
		record.ItemCount++
		if item.Universe != "" {
			universes[item.Universe] = struct{}{}
		}
		if representative == nil ||
			item.Number < representative.Number ||
			(item.Number == representative.Number && item.ID < representative.ID) {
			candidate := cloneCatalogItem(item)
			representative = &candidate
		}
		addEntryToSummary(&record.MemberSummary, store.entries[userID][item.ID])
	}
	for universe := range universes {
		record.UniverseOptions = append(record.UniverseOptions, universe)
	}
	sort.Strings(record.UniverseOptions)
	if representative != nil {
		record.Artwork = representative.Artwork
	}
	return record, nil
}

type rankedMatchRequest struct {
	request     MatchRequest
	cappedSince int64
	stale       bool
}

func (store *MemoryStore) preferredFulfillableRequestLocked(
	availableIDs []string,
	currentEntries map[string]domain.CollectionEntry,
	counterpartID string,
	receiveItem domain.CatalogItem,
	scope MatchScope,
	rankingAt time.Time,
) *rankedMatchRequest {
	var preferred *rankedMatchRequest
	for _, availableID := range availableIDs {
		availableItem := store.items[availableID]
		if !matchScopeAllows(scope, availableItem.CatalogID, receiveItem.CatalogID) {
			continue
		}
		theirWant, exists := store.wants[availableID][counterpartID]
		if !exists {
			continue
		}
		quantity := minimum(
			currentEntries[availableID].Available,
			theirWant.Wanted,
		)
		if quantity <= 0 {
			continue
		}
		cappedSince, stale := matchWaitRank(
			store.timings[counterpartID][availableID].wantedSince,
			store.lastSeen[counterpartID],
			rankingAt,
		)
		candidate := &rankedMatchRequest{
			request: MatchRequest{
				CatalogItem: store.catalogItemRecordLocked(availableItem),
				Quantity:    quantity,
				Fulfillable: true,
			},
			cappedSince: cappedSince,
			stale:       stale,
		}
		if preferred == nil || rankedRequestLess(candidate, preferred) {
			preferred = candidate
		}
	}
	return preferred
}

func (store *MemoryStore) preferredUnfulfillableRequestLocked(
	currentEntries map[string]domain.CollectionEntry,
	counterpartID string,
	receiveItem domain.CatalogItem,
	rankingAt time.Time,
) *rankedMatchRequest {
	var preferred *rankedMatchRequest
	for itemID, entry := range store.entries[counterpartID] {
		if entry.Wanted <= 0 || itemID == receiveItem.ID {
			continue
		}
		item, exists := store.publishedItemLocked(itemID)
		if !exists || currentEntries[itemID].Available > 0 {
			continue
		}
		cappedSince, stale := matchWaitRank(
			store.timings[counterpartID][itemID].wantedSince,
			store.lastSeen[counterpartID],
			rankingAt,
		)
		candidate := &rankedMatchRequest{
			request: MatchRequest{
				CatalogItem: store.catalogItemRecordLocked(item),
				Quantity:    0,
				Fulfillable: false,
			},
			cappedSince: cappedSince,
			stale:       stale,
		}
		if preferred == nil || rankedRequestLess(candidate, preferred) {
			preferred = candidate
		}
	}
	return preferred
}

func rankedRequestLess(left, right *rankedMatchRequest) bool {
	if left.cappedSince != right.cappedSince {
		return left.cappedSince < right.cappedSince
	}
	if left.stale != right.stale {
		return !left.stale
	}
	return left.request.CatalogItem.Item.ID < right.request.CatalogItem.Item.ID
}

func matchWaitRank(
	since time.Time,
	lastSeen time.Time,
	rankingAt time.Time,
) (int64, bool) {
	if since.IsZero() {
		since = rankingAt
	}
	capCutoff := rankingAt.Add(-MatchWaitCap)
	cappedSince := since
	if cappedSince.Before(capCutoff) {
		cappedSince = capCutoff
	}
	staleCutoff := rankingAt.Add(-MatchStaleAfter)
	stale := since.Before(staleCutoff) &&
		(lastSeen.IsZero() || lastSeen.Before(staleCutoff))
	return cappedSince.Unix(), stale
}

func (store *MemoryStore) catalogItemRecordLocked(item domain.CatalogItem) CatalogItemRecord {
	catalog := store.catalogs[item.CatalogID]
	return CatalogItemRecord{
		Brand:   store.brands[catalog.BrandID],
		Catalog: catalog,
		Item:    cloneCatalogItem(item),
	}
}

func (store *MemoryStore) collectionRecordLocked(
	item domain.CatalogItem,
	entry domain.CollectionEntry,
) CollectionRecord {
	return CollectionRecord{
		CatalogItem: store.catalogItemRecordLocked(item),
		Owned:       entry.Owned,
		Available:   entry.Available,
		Wanted:      entry.Wanted,
	}
}

func (store *MemoryStore) publishedCatalogLocked(id string) (domain.Catalog, bool) {
	catalog, exists := store.catalogs[id]
	if !exists || catalog.Status == domain.PublicationDraft {
		return domain.Catalog{}, false
	}
	brand, exists := store.brands[catalog.BrandID]
	if !exists || brand.Status == domain.PublicationDraft {
		return domain.Catalog{}, false
	}
	return catalog, true
}

func (store *MemoryStore) publishedItemLocked(id string) (domain.CatalogItem, bool) {
	item, exists := store.items[id]
	if !exists || item.Status == domain.PublicationDraft {
		return domain.CatalogItem{}, false
	}
	if _, exists := store.publishedCatalogLocked(item.CatalogID); !exists {
		return domain.CatalogItem{}, false
	}
	return item, true
}

func (store *MemoryStore) brandUsedByOtherCatalogLocked(brandID, catalogID string) bool {
	for _, catalog := range store.catalogs {
		if catalog.BrandID == brandID && catalog.ID != catalogID {
			return true
		}
	}
	return false
}

func normalizeCatalogQuery(query CatalogQuery) (CatalogQuery, error) {
	const operation = "list catalogs"
	if query.UserID == "" {
		return CatalogQuery{}, invalidArgument(operation, "user_id", "is required")
	}
	search, err := normalizeSearch(operation, query.Search)
	if err != nil {
		return CatalogQuery{}, err
	}
	query.Search = search
	switch query.PublicationStatus {
	case "":
		query.PublicationStatus = PublicationFilterActive
	case PublicationFilterActive, PublicationFilterRetired, PublicationFilterAll:
	default:
		return CatalogQuery{}, invalidArgument(
			operation,
			"publication_status",
			"must be active, retired, or all",
		)
	}
	query.Limit, err = normalizeLimit(
		operation,
		query.Limit,
		DefaultCatalogPageSize,
		MaxCatalogPageSize,
	)
	if err != nil {
		return CatalogQuery{}, err
	}
	if query.After != nil && (query.After.SortOrder <= 0 || query.After.CatalogID == "") {
		return CatalogQuery{}, invalidArgument(operation, "after", "is incomplete")
	}
	return query, nil
}

func normalizeCollectionQuery(query CollectionQuery) (CollectionQuery, error) {
	const operation = "list collection page"
	if query.UserID == "" {
		return CollectionQuery{}, invalidArgument(operation, "user_id", "is required")
	}
	if query.CatalogID == "" {
		return CollectionQuery{}, invalidArgument(operation, "catalog_id", "is required")
	}
	search, err := normalizeSearch(operation, query.Search)
	if err != nil {
		return CollectionQuery{}, err
	}
	query.Search = search
	switch query.State {
	case "":
		query.State = CollectionStateAll
	case CollectionStateAll, CollectionStateOwned, CollectionStateAvailable, CollectionStateWanted:
	default:
		return CollectionQuery{}, invalidArgument(
			operation,
			"state",
			"must be all, owned, available, or wanted",
		)
	}
	query.Limit, err = normalizeLimit(
		operation,
		query.Limit,
		DefaultCollectionPageSize,
		MaxCollectionPageSize,
	)
	if err != nil {
		return CollectionQuery{}, err
	}
	if query.After != nil && (query.After.ItemNumber <= 0 || query.After.ItemID == "") {
		return CollectionQuery{}, invalidArgument(operation, "after", "is incomplete")
	}
	return query, nil
}

func normalizeMatchQuery(query MatchQuery) (MatchQuery, error) {
	const operation = "find matches"
	if query.UserID == "" {
		return MatchQuery{}, invalidArgument(operation, "user_id", "is required")
	}
	switch query.Scope {
	case "":
		query.Scope = MatchScopeAll
	case MatchScopeAll, MatchScopeSameCatalog, MatchScopeCrossCatalog:
	default:
		return MatchQuery{}, invalidArgument(
			operation,
			"scope",
			"must be all, same_catalog, or cross_catalog",
		)
	}
	switch query.Readiness {
	case "":
		query.Readiness = MatchReadinessActionable
	case MatchReadinessActionable, MatchReadinessUnfulfillable:
	default:
		return MatchQuery{}, invalidArgument(
			operation,
			"readiness",
			"must be actionable or unfulfillable",
		)
	}
	if query.Readiness == MatchReadinessUnfulfillable &&
		(query.GiveCatalogID != "" || query.Scope != MatchScopeAll) {
		return MatchQuery{}, invalidArgument(
			operation,
			"readiness",
			"unfulfillable cannot be combined with give_catalog_id or a reciprocal scope",
		)
	}
	var err error
	query.Limit, err = normalizeLimit(
		operation,
		query.Limit,
		DefaultMatchPageSize,
		MaxMatchPageSize,
	)
	if err != nil {
		return MatchQuery{}, err
	}
	if query.After != nil &&
		(query.After.RequestClass < 0 ||
			query.After.RequestClass > 1 ||
			query.After.CappedSince <= 0 ||
			(query.After.RequestClass == 0 &&
				query.After.RequestItemID == "") ||
			(query.After.RequestClass == 1 &&
				query.After.RequestItemID != "") ||
			query.After.CounterpartID == "" ||
			query.After.ReceiveItemID == "") {
		return MatchQuery{}, invalidArgument(operation, "after", "is incomplete")
	}
	if query.After != nil && query.RankingAt.IsZero() {
		return MatchQuery{}, invalidArgument(
			operation,
			"ranking_at",
			"is required for a continuation",
		)
	}
	return query, nil
}

func normalizeSearch(operation, value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", invalidArgument(operation, "q", "must be valid UTF-8")
	}
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > MaxSearchRunes {
		return "", invalidArgument(
			operation,
			"q",
			fmt.Sprintf("must not exceed %d Unicode code points", MaxSearchRunes),
		)
	}
	return strings.ToLower(value), nil
}

func normalizeLimit(operation string, value, defaultValue, maximum int) (int, error) {
	if value == 0 {
		return defaultValue, nil
	}
	if value < 1 || value > maximum {
		return 0, invalidArgument(
			operation,
			"limit",
			fmt.Sprintf("must be from 1 through %d", maximum),
		)
	}
	return value, nil
}

func catalogStatusMatches(status domain.PublicationStatus, filter PublicationFilter) bool {
	switch filter {
	case PublicationFilterActive:
		return status == domain.PublicationActive
	case PublicationFilterRetired:
		return status == domain.PublicationRetired
	case PublicationFilterAll:
		return status == domain.PublicationActive || status == domain.PublicationRetired
	default:
		return false
	}
}

func collectionStateMatches(entry domain.CollectionEntry, state CollectionState) bool {
	switch state {
	case CollectionStateAll:
		return true
	case CollectionStateOwned:
		return entry.Owned > 0
	case CollectionStateAvailable:
		return entry.Available > 0
	case CollectionStateWanted:
		return entry.Wanted > 0
	default:
		return false
	}
}

func matchScopeAllows(scope MatchScope, giveCatalogID, receiveCatalogID string) bool {
	switch scope {
	case MatchScopeAll:
		return true
	case MatchScopeSameCatalog:
		return giveCatalogID == receiveCatalogID
	case MatchScopeCrossCatalog:
		return giveCatalogID != receiveCatalogID
	default:
		return false
	}
}

func catalogAfter(catalog domain.Catalog, after CatalogPosition) bool {
	return catalog.SortOrder > after.SortOrder ||
		(catalog.SortOrder == after.SortOrder && catalog.ID > after.CatalogID)
}

func itemAfter(item domain.CatalogItem, after CollectionPosition) bool {
	return item.Number > after.ItemNumber ||
		(item.Number == after.ItemNumber && item.ID > after.ItemID)
}

func matchAfter(position, after MatchPosition) bool {
	return matchPositionLess(after, position)
}

func matchPositionLess(left, right MatchPosition) bool {
	if left.RequestClass != right.RequestClass {
		return left.RequestClass < right.RequestClass
	}
	if left.CappedSince != right.CappedSince {
		return left.CappedSince < right.CappedSince
	}
	if left.Stale != right.Stale {
		return !left.Stale
	}
	if left.RequestItemID != right.RequestItemID {
		return left.RequestItemID < right.RequestItemID
	}
	if left.CounterpartID != right.CounterpartID {
		return left.CounterpartID < right.CounterpartID
	}
	return left.ReceiveItemID < right.ReceiveItemID
}

func containsFold(value, normalizedSearch string) bool {
	return strings.Contains(strings.ToLower(value), normalizedSearch)
}

func sortedStringsContain(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

func addEntryToSummary(summary *MemberSummary, entry domain.CollectionEntry) {
	summary.OwnedQuantity += entry.Owned
	summary.AvailableQuantity += entry.Available
	summary.WantedQuantity += entry.Wanted
	if entry.Owned > 0 {
		summary.OwnedDesigns++
	}
	if entry.Available > 0 {
		summary.AvailableDesigns++
	}
	if entry.Wanted > 0 {
		summary.WantedDesigns++
	}
}

func (store *MemoryStore) removeEntryIndexesLocked(entry domain.CollectionEntry) {
	if entry.Available > 0 {
		delete(store.offers[entry.ItemID], entry.UserID)
		if len(store.offers[entry.ItemID]) == 0 {
			delete(store.offers, entry.ItemID)
		}
	}
	if entry.Wanted > 0 {
		delete(store.wants[entry.ItemID], entry.UserID)
		if len(store.wants[entry.ItemID]) == 0 {
			delete(store.wants, entry.ItemID)
		}
	}
}

func (store *MemoryStore) addEntryIndexesLocked(entry domain.CollectionEntry) {
	if entry.Available > 0 {
		if store.offers[entry.ItemID] == nil {
			store.offers[entry.ItemID] = make(map[string]domain.CollectionEntry)
		}
		store.offers[entry.ItemID][entry.UserID] = entry
	}
	if entry.Wanted > 0 {
		if store.wants[entry.ItemID] == nil {
			store.wants[entry.ItemID] = make(map[string]domain.CollectionEntry)
		}
		store.wants[entry.ItemID][entry.UserID] = entry
	}
}

func emptyMatchPage(limit int) MatchPage {
	return MatchPage{Matches: make([]MatchRecord, 0), Limit: limit}
}

func minimum(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func matchID(userID, counterpartID, receiveID string) string {
	sum := sha256.Sum256(
		[]byte(userID + "\x00" + counterpartID + "\x00" + receiveID),
	)
	return "match-" + hex.EncodeToString(sum[:12])
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func cloneCatalogItem(item domain.CatalogItem) domain.CatalogItem {
	if item.Retirement != nil {
		retirement := *item.Retirement
		item.Retirement = &retirement
	}
	return item
}

func cloneManifest(manifest domain.CatalogManifest) domain.CatalogManifest {
	manifest.Items = append([]domain.CatalogItem(nil), manifest.Items...)
	for index := range manifest.Items {
		manifest.Items[index] = cloneCatalogItem(manifest.Items[index])
	}
	return manifest
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

var (
	_ HealthChecker    = (*MemoryStore)(nil)
	_ UserStore        = (*MemoryStore)(nil)
	_ CatalogReader    = (*MemoryStore)(nil)
	_ CollectionReader = (*MemoryStore)(nil)
	_ CollectionWriter = (*MemoryStore)(nil)
	_ MatchFinder      = (*MemoryStore)(nil)
	_ ManifestImporter = (*MemoryStore)(nil)
)
