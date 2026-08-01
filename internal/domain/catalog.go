package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// CatalogManifestSchemaVersion is the only manifest shape understood by
	// this version of the application.
	CatalogManifestSchemaVersion = 1

	// MaxCatalogItems keeps catalog import and validation work explicitly
	// bounded while leaving ample room for finite collectible sets.
	MaxCatalogItems = 10_000

	maxCatalogNameBytes = 200
	maxEditionBytes     = 100
	maxLabelBytes       = 100
	maxRightsTextBytes  = 500
	maxReferenceBytes   = 256
	maxURLBytes         = 2_048
)

// PublicationStatus records whether catalog data is visible or retained only
// for history. A missing status is invalid so retirement is never inferred from
// an omitted item.
type PublicationStatus string

const (
	PublicationDraft   PublicationStatus = "draft"
	PublicationActive  PublicationStatus = "active"
	PublicationRetired PublicationStatus = "retired"
)

// RightsStatus describes why an artwork reference may be published, or why the
// local placeholder must be used.
type RightsStatus string

const (
	RightsOriginal         RightsStatus = "original"
	RightsLicensed         RightsStatus = "licensed"
	RightsPermissionedLink RightsStatus = "permissioned-link"
	RightsSourceHostedLink RightsStatus = "source-hosted-link"
	RightsUnknown          RightsStatus = "unknown"
	RightsRestricted       RightsStatus = "restricted"
)

// Brand is the issuer or promoter responsible for one or more catalogs.
type Brand struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Status PublicationStatus `json:"status"`
}

// Validate verifies a brand's stable identity and bounded display data.
func (b Brand) Validate() error {
	if !validSlug(b.ID) {
		return ValidationError{Field: "id", Reason: "must be a lowercase slug"}
	}
	if err := validateRequiredText("name", b.Name, maxCatalogNameBytes); err != nil {
		return err
	}
	if !b.Status.valid() {
		return ValidationError{Field: "status", Reason: "must be draft, active, or retired"}
	}
	return nil
}

// Catalog is one finite edition owned by a brand.
type Catalog struct {
	ID        string            `json:"id"`
	BrandID   string            `json:"brand_id"`
	Name      string            `json:"name"`
	Edition   string            `json:"edition,omitempty"`
	Status    PublicationStatus `json:"status"`
	SortOrder int               `json:"sort_order"`
}

// Validate verifies a catalog's identity, ownership, and display fields.
func (c Catalog) Validate() error {
	if !validSlug(c.ID) {
		return ValidationError{Field: "id", Reason: "must be a lowercase slug"}
	}
	if !validSlug(c.BrandID) {
		return ValidationError{Field: "brand_id", Reason: "must be a lowercase slug"}
	}
	if err := validateRequiredText("name", c.Name, maxCatalogNameBytes); err != nil {
		return err
	}
	if err := validateOptionalText("edition", c.Edition, maxEditionBytes); err != nil {
		return err
	}
	if !c.Status.valid() {
		return ValidationError{Field: "status", Reason: "must be draft, active, or retired"}
	}
	if c.SortOrder <= 0 {
		return ValidationError{Field: "sort_order", Reason: "must be positive"}
	}
	return nil
}

// ArtworkRights records the reviewed basis for showing an artwork reference.
type ArtworkRights struct {
	Status              RightsStatus `json:"status"`
	License             string       `json:"license,omitempty"`
	Attribution         string       `json:"attribution,omitempty"`
	PermissionReference string       `json:"permission_reference,omitempty"`
}

// Validate verifies bounded rights metadata. Cross-field publication rules are
// enforced by Artwork.Validate.
func (r ArtworkRights) Validate() error {
	if !r.Status.valid() {
		return ValidationError{
			Field:  "status",
			Reason: "must be original, licensed, permissioned-link, source-hosted-link, unknown, or restricted",
		}
	}
	if err := validateOptionalText("license", r.License, maxRightsTextBytes); err != nil {
		return err
	}
	if err := validateOptionalText("attribution", r.Attribution, maxRightsTextBytes); err != nil {
		return err
	}
	if err := validateOptionalText(
		"permission_reference",
		r.PermissionReference,
		maxReferenceBytes,
	); err != nil {
		return err
	}
	return nil
}

// ArtworkProvenance records where an artwork decision came from and when it was
// reviewed. SHA256 is used only when bytes were reviewed or packaged.
type ArtworkProvenance struct {
	SourcePageURL   string `json:"source_page_url,omitempty"`
	RetrievedAt     string `json:"retrieved_at,omitempty"`
	ReviewedAt      string `json:"reviewed_at"`
	ReviewReference string `json:"review_reference"`
	Transformation  string `json:"transformation"`
	SHA256          string `json:"sha256,omitempty"`
}

// Validate verifies safe source metadata and canonical review dates.
func (p ArtworkProvenance) Validate() error {
	if p.SourcePageURL != "" {
		if err := validateHTTPSURL("source_page_url", p.SourcePageURL); err != nil {
			return err
		}
		if err := validateDate("retrieved_at", p.RetrievedAt); err != nil {
			return err
		}
	} else if p.RetrievedAt != "" {
		return ValidationError{Field: "retrieved_at", Reason: "requires source_page_url"}
	}
	if err := validateDate("reviewed_at", p.ReviewedAt); err != nil {
		return err
	}
	if err := validateRequiredText(
		"review_reference",
		p.ReviewReference,
		maxReferenceBytes,
	); err != nil {
		return err
	}
	if err := validateRequiredText(
		"transformation",
		p.Transformation,
		maxReferenceBytes,
	); err != nil {
		return err
	}
	if p.SHA256 != "" && !validSHA256(p.SHA256) {
		return ValidationError{Field: "sha256", Reason: "must be 64 lowercase hexadecimal characters"}
	}
	return nil
}

// Artwork is either one reviewed HTTPS reference or an explicit instruction to
// use the application's local placeholder.
type Artwork struct {
	AssetURL    string            `json:"asset_url,omitempty"`
	Placeholder bool              `json:"placeholder"`
	Rights      ArtworkRights     `json:"rights"`
	Provenance  ArtworkProvenance `json:"provenance"`
}

// Validate enforces the publication behavior implied by the rights decision.
func (a Artwork) Validate() error {
	if err := a.Rights.Validate(); err != nil {
		return prefixValidation("rights", err)
	}
	if err := a.Provenance.Validate(); err != nil {
		return prefixValidation("provenance", err)
	}
	if a.AssetURL != "" {
		if err := validateHTTPSURL("asset_url", a.AssetURL); err != nil {
			return err
		}
	}

	switch a.Rights.Status {
	case RightsUnknown, RightsRestricted:
		if !a.Placeholder {
			return ValidationError{Field: "placeholder", Reason: "must be true for unknown or restricted rights"}
		}
		if a.AssetURL != "" {
			return ValidationError{Field: "asset_url", Reason: "must be empty for unknown or restricted rights"}
		}
		if a.Rights.PermissionReference != "" {
			return ValidationError{
				Field:  "rights.permission_reference",
				Reason: "must be empty for unknown or restricted rights",
			}
		}
		if a.Rights.Status == RightsUnknown && a.Rights.License != "" {
			return ValidationError{Field: "rights.license", Reason: "must be empty when rights are unknown"}
		}
	case RightsOriginal, RightsLicensed, RightsPermissionedLink, RightsSourceHostedLink:
		if a.Placeholder {
			return ValidationError{Field: "placeholder", Reason: "must be false for publishable artwork"}
		}
		if a.AssetURL == "" {
			return ValidationError{Field: "asset_url", Reason: "is required for publishable artwork"}
		}
		if a.Provenance.SourcePageURL == "" {
			return ValidationError{Field: "provenance.source_page_url", Reason: "is required for publishable artwork"}
		}
		if a.Provenance.RetrievedAt == "" {
			return ValidationError{Field: "provenance.retrieved_at", Reason: "is required for an asset URL"}
		}
		if a.Rights.Attribution == "" {
			return ValidationError{Field: "rights.attribution", Reason: "is required for publishable artwork"}
		}
		if (a.Rights.Status == RightsOriginal || a.Rights.Status == RightsLicensed ||
			a.Rights.Status == RightsSourceHostedLink) && a.Rights.License == "" {
			return ValidationError{Field: "rights.license", Reason: "is required for this rights status"}
		}
		if a.Rights.Status == RightsPermissionedLink && a.Rights.PermissionReference == "" {
			return ValidationError{
				Field:  "rights.permission_reference",
				Reason: "is required for permissioned-link artwork",
			}
		}
		if a.Rights.Status != RightsPermissionedLink && a.Rights.PermissionReference != "" {
			return ValidationError{
				Field:  "rights.permission_reference",
				Reason: "is only valid for permissioned-link artwork",
			}
		}
	}
	return nil
}

// ItemRetirement makes removal explicit while preserving the immutable item ID.
type ItemRetirement struct {
	EffectiveDate string `json:"effective_date"`
	Reason        string `json:"reason"`
}

// Validate verifies an explicit, reviewable retirement directive.
func (r ItemRetirement) Validate() error {
	if err := validateDate("effective_date", r.EffectiveDate); err != nil {
		return err
	}
	return validateRequiredText("reason", r.Reason, maxRightsTextBytes)
}

// CatalogItem is one item in one catalog. ID is globally unique and immutable;
// CatalogID remains explicit ownership metadata.
type CatalogItem struct {
	ID         string            `json:"id"`
	CatalogID  string            `json:"catalog_id"`
	Number     int               `json:"number"`
	Name       string            `json:"name"`
	Universe   string            `json:"universe,omitempty"`
	Variant    string            `json:"variant,omitempty"`
	Status     PublicationStatus `json:"status"`
	Retirement *ItemRetirement   `json:"retirement,omitempty"`
	Artwork    Artwork           `json:"artwork"`
}

// Validate verifies item identity, catalog-local ordering, display data,
// explicit retirement, and artwork policy.
func (i CatalogItem) Validate() error {
	if !validSlug(i.ID) {
		return ValidationError{Field: "id", Reason: "must be a globally unique lowercase slug"}
	}
	if !validSlug(i.CatalogID) {
		return ValidationError{Field: "catalog_id", Reason: "must be a lowercase slug"}
	}
	if i.Number <= 0 {
		return ValidationError{Field: "number", Reason: "must be positive"}
	}
	if err := validateRequiredText("name", i.Name, maxCatalogNameBytes); err != nil {
		return err
	}
	if err := validateOptionalText("universe", i.Universe, maxLabelBytes); err != nil {
		return err
	}
	if err := validateOptionalText("variant", i.Variant, maxLabelBytes); err != nil {
		return err
	}
	if !i.Status.valid() {
		return ValidationError{Field: "status", Reason: "must be draft, active, or retired"}
	}
	if i.Status == PublicationRetired {
		if i.Retirement == nil {
			return ValidationError{Field: "retirement", Reason: "is required when status is retired"}
		}
		if err := i.Retirement.Validate(); err != nil {
			return prefixValidation("retirement", err)
		}
	} else if i.Retirement != nil {
		return ValidationError{Field: "retirement", Reason: "is only valid when status is retired"}
	}
	if err := i.Artwork.Validate(); err != nil {
		return prefixValidation("artwork", err)
	}
	return nil
}

// CatalogManifest is one independently versioned, complete catalog snapshot.
// Items may be added or explicitly retired, but must not silently disappear.
type CatalogManifest struct {
	SchemaVersion   int           `json:"schema_version"`
	ManifestVersion int           `json:"manifest_version"`
	Brand           Brand         `json:"brand"`
	Catalog         Catalog       `json:"catalog"`
	Items           []CatalogItem `json:"items"`
}

// Validate verifies one complete manifest. Set-wide global ID checks belong to
// the manifest loader because one manifest cannot see other catalogs.
func (m CatalogManifest) Validate() error {
	if m.SchemaVersion != CatalogManifestSchemaVersion {
		return ValidationError{
			Field:  "schema_version",
			Reason: fmt.Sprintf("must equal %d", CatalogManifestSchemaVersion),
		}
	}
	if m.ManifestVersion <= 0 {
		return ValidationError{Field: "manifest_version", Reason: "must be positive"}
	}
	if err := m.Brand.Validate(); err != nil {
		return prefixValidation("brand", err)
	}
	if err := m.Catalog.Validate(); err != nil {
		return prefixValidation("catalog", err)
	}
	if m.Catalog.BrandID != m.Brand.ID {
		return ValidationError{Field: "catalog.brand_id", Reason: "must match brand.id"}
	}
	if len(m.Items) == 0 {
		return ValidationError{Field: "items", Reason: "must contain at least one item"}
	}
	if len(m.Items) > MaxCatalogItems {
		return ValidationError{
			Field:  "items",
			Reason: fmt.Sprintf("must not contain more than %d items", MaxCatalogItems),
		}
	}

	ids := make(map[string]struct{}, len(m.Items))
	numbers := make(map[int]struct{}, len(m.Items))
	publishedItems := 0
	for index, item := range m.Items {
		prefix := fmt.Sprintf("items[%d]", index)
		if err := item.Validate(); err != nil {
			return prefixValidation(prefix, err)
		}
		if item.CatalogID != m.Catalog.ID {
			return ValidationError{Field: prefix + ".catalog_id", Reason: "must match catalog.id"}
		}
		if _, duplicate := ids[item.ID]; duplicate {
			return ValidationError{Field: prefix + ".id", Reason: "must be globally unique"}
		}
		ids[item.ID] = struct{}{}
		if _, duplicate := numbers[item.Number]; duplicate {
			return ValidationError{Field: prefix + ".number", Reason: "must be unique within the catalog"}
		}
		numbers[item.Number] = struct{}{}
		if item.Status != PublicationDraft {
			publishedItems++
		}
	}
	if m.Catalog.Status != PublicationDraft && publishedItems == 0 {
		return ValidationError{
			Field:  "items",
			Reason: "must contain a published item when the catalog is published",
		}
	}
	return nil
}

// ValidateSuccessor verifies that m is a later complete version of previous.
// Existing items must remain under the same ID; retirement is represented by a
// retained item with retired status, never by omission.
func (m CatalogManifest) ValidateSuccessor(previous CatalogManifest) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("successor manifest: %w", err)
	}
	if m.Brand.ID != previous.Brand.ID {
		return ValidationError{Field: "brand.id", Reason: "must not change between manifest versions"}
	}
	if m.Catalog.ID != previous.Catalog.ID {
		return ValidationError{Field: "catalog.id", Reason: "must not change between manifest versions"}
	}
	if m.Catalog.SortOrder != previous.Catalog.SortOrder {
		return ValidationError{
			Field:  "catalog.sort_order",
			Reason: "must not change between manifest versions",
		}
	}
	if m.ManifestVersion <= previous.ManifestVersion {
		return ValidationError{Field: "manifest_version", Reason: "must increase"}
	}
	if previous.Brand.Status == PublicationRetired && m.Brand.Status != PublicationRetired {
		return ValidationError{Field: "brand.status", Reason: "must not reactivate a retired brand"}
	}
	if previous.Catalog.Status == PublicationRetired && m.Catalog.Status != PublicationRetired {
		return ValidationError{Field: "catalog.status", Reason: "must not reactivate a retired catalog"}
	}

	currentItems := make(map[string]CatalogItem, len(m.Items))
	for _, item := range m.Items {
		currentItems[item.ID] = item
	}
	for _, oldItem := range previous.Items {
		currentItem, exists := currentItems[oldItem.ID]
		if !exists {
			return ValidationError{
				Field:  "items",
				Reason: fmt.Sprintf("item %q must remain and be explicitly retired", oldItem.ID),
			}
		}
		if oldItem.Status == PublicationRetired && currentItem.Status != PublicationRetired {
			return ValidationError{
				Field:  "items",
				Reason: fmt.Sprintf("retired item %q must not be reactivated", oldItem.ID),
			}
		}
		if currentItem.Number != oldItem.Number {
			return ValidationError{
				Field:  "items",
				Reason: fmt.Sprintf("item %q must keep number %d", oldItem.ID, oldItem.Number),
			}
		}
	}
	return nil
}

func (s PublicationStatus) valid() bool {
	return s == PublicationDraft || s == PublicationActive || s == PublicationRetired
}

func (s RightsStatus) valid() bool {
	switch s {
	case RightsOriginal, RightsLicensed, RightsPermissionedLink,
		RightsSourceHostedLink, RightsUnknown, RightsRestricted:
		return true
	default:
		return false
	}
}

func validateRequiredText(field, value string, maxBytes int) error {
	if value == "" || strings.TrimSpace(value) == "" {
		return ValidationError{Field: field, Reason: "must not be empty"}
	}
	return validateText(field, value, maxBytes)
}

func validateOptionalText(field, value string, maxBytes int) error {
	if value == "" {
		return nil
	}
	return validateText(field, value, maxBytes)
}

func validateText(field, value string, maxBytes int) error {
	if !utf8.ValidString(value) {
		return ValidationError{Field: field, Reason: "must be valid UTF-8"}
	}
	if strings.TrimSpace(value) != value {
		return ValidationError{Field: field, Reason: "must not have leading or trailing whitespace"}
	}
	if len(value) > maxBytes {
		return ValidationError{Field: field, Reason: fmt.Sprintf("must not exceed %d bytes", maxBytes)}
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ValidationError{Field: field, Reason: "must not contain control characters"}
		}
	}
	return nil
}

func validateHTTPSURL(field, raw string) error {
	if err := validateRequiredText(field, raw, maxURLBytes); err != nil {
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return ValidationError{
			Field:  field,
			Reason: "must be an absolute HTTPS URL without credentials or a fragment",
		}
	}
	return nil
}

func validateDate(field, value string) error {
	if value == "" {
		return ValidationError{Field: field, Reason: "must be an ISO 8601 date"}
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil || parsed.Format(time.DateOnly) != value {
		return ValidationError{Field: field, Reason: "must be an ISO 8601 date"}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func prefixValidation(prefix string, err error) error {
	var validationError ValidationError
	if errors.As(err, &validationError) {
		return ValidationError{
			Field:  prefix + "." + validationError.Field,
			Reason: validationError.Reason,
		}
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
