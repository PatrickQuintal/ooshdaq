package qualification

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
)

func TestFullConfigMatchesAcceptedEnvelope(t *testing.T) {
	config := FullConfig(filepath.Join(t.TempDir(), "qualification.sqlite"))
	if err := config.validate(); err != nil {
		t.Fatalf("FullConfig validate: %v", err)
	}
	if !config.isFixedReleaseShape() {
		t.Fatal("FullConfig does not meet the fixed release shape")
	}
	if !config.isFixedReleaseWorkload() {
		t.Fatal("FullConfig does not meet the fixed release workload")
	}
	if config.MemberThinkTime != FullMemberThinkTime {
		t.Fatalf(
			"member think time = %v, want %v",
			config.MemberThinkTime,
			FullMemberThinkTime,
		)
	}
	changedPacing := config
	changedPacing.MemberThinkTime++
	if changedPacing.isFixedReleaseWorkload() {
		t.Fatal("changed member pacing still claims the fixed release workload")
	}
	if got := config.CollectionRows / config.Users; got != 8 {
		t.Fatalf("rows per user = %d, want 8", got)
	}
}

func TestGeneratedFixtureIsDeterministicAndRedistributionSafe(t *testing.T) {
	config := SmokeConfig(filepath.Join(t.TempDir(), "unused.sqlite"))
	firstDigest := sha256.New()
	first, _, firstItems, err := generatedManifests(config, firstDigest)
	if err != nil {
		t.Fatalf("generatedManifests first: %v", err)
	}
	secondDigest := sha256.New()
	second, _, secondItems, err := generatedManifests(config, secondDigest)
	if err != nil {
		t.Fatalf("generatedManifests second: %v", err)
	}
	if string(firstDigest.Sum(nil)) != string(secondDigest.Sum(nil)) {
		t.Fatal("generated manifest digest is not deterministic")
	}
	if len(second) != len(first) {
		t.Fatalf("generated catalogs = %d/%d", len(first), len(second))
	}
	if len(firstItems) != config.Catalogs*config.ItemsPerCatalog ||
		len(secondItems) != len(firstItems) {
		t.Fatalf("generated items = %d/%d", len(firstItems), len(secondItems))
	}
	manifests := make([]domain.CatalogManifest, 0, len(first))
	for _, loaded := range first {
		manifest := loaded.Manifest
		manifests = append(manifests, manifest)
		if manifest.Brand.Status != domain.PublicationActive {
			t.Fatalf("brand %q is not active", manifest.Brand.ID)
		}
		for _, item := range manifest.Items {
			if !item.Artwork.Placeholder ||
				item.Artwork.AssetURL != "" ||
				item.Artwork.Rights.Status != domain.RightsUnknown ||
				item.Artwork.Provenance.SourcePageURL != "" {
				t.Fatalf("item %q is not a rights-safe placeholder", item.ID)
			}
		}
	}
	if err := catalogdata.ValidateManifestSet(manifests); err != nil {
		t.Fatalf("ValidateManifestSet: %v", err)
	}
	pairs, same, cross, err := reciprocalPairs(config, firstItems)
	if err != nil {
		t.Fatalf("reciprocalPairs: %v", err)
	}
	if len(pairs) != config.Users/2 || same == 0 || cross == 0 {
		t.Fatalf("pair shape len=%d same=%d cross=%d", len(pairs), same, cross)
	}
}

func TestSmokeRunExercisesSQLiteQualificationPath(t *testing.T) {
	config := SmokeConfig(filepath.Join(t.TempDir(), "qualification.sqlite"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	report, err := Run(ctx, config)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Fixture.CatalogCount != config.Catalogs ||
		report.Fixture.UserCount != config.Users ||
		report.Fixture.CollectionRowCount != config.CollectionRows {
		t.Fatalf("fixture report = %+v", report.Fixture)
	}
	if !report.Fixture.CrossCatalogJourneyVerified ||
		!report.Fixture.SameCatalogJourneyVerified ||
		!report.Fixture.OneWayJourneyVerified ||
		!report.Fixture.UnfulfillableJourneyVerified {
		t.Fatalf("possible-swap journeys = %+v", report.Fixture)
	}
	if report.FixtureVersion != "qualification-v3" {
		t.Fatalf("fixture version = %q, want qualification-v3", report.FixtureVersion)
	}
	if report.Workload.TotalAttempts < uint64(len(workloadMix)) {
		t.Fatalf("workload attempts = %d, want at least %d", report.Workload.TotalAttempts, len(workloadMix))
	}
	if report.Workload.Operations[operationMatchesActionable].Successes == 0 ||
		report.Workload.Operations[operationMatchesUnfulfillable].Successes == 0 {
		t.Fatalf("split match workload = %+v", report.Workload.Operations)
	}
	if report.Workload.UnexpectedErrors != 0 ||
		report.Workload.BusyErrors != 0 ||
		report.Workload.LimitViolations != 0 {
		t.Fatalf("workload errors = %+v", report.Workload)
	}
	if !report.Gates.ExactRequestMix ||
		!report.Gates.PossibleSwapJourneysPassed ||
		!report.Gates.QueryPlanPassed ||
		!report.Gates.ReadinessIntegrityPassed ||
		!report.Gates.CheckpointPassed {
		t.Fatalf("smoke gates = %+v", report.Gates)
	}
	if report.Gates.FixedReleaseShape ||
		report.Gates.FixedReleaseWorkload ||
		report.Gates.FormalReleaseGatePassed {
		t.Fatalf("smoke run claimed release qualification: %+v", report.Gates)
	}
	if len(report.Verification.RequiredIndexes) == 0 ||
		len(report.Verification.QueryPlanDetails) == 0 {
		t.Fatalf("query plan evidence = %+v", report.Verification)
	}
}

func TestLatencyHistogramAndExactMix(t *testing.T) {
	histogram := newLatencyHistogram()
	for _, duration := range []time.Duration{
		100 * time.Microsecond,
		time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		time.Second,
	} {
		histogram.observe(duration)
	}
	if got := histogram.percentile(50, 100); got != 10*time.Millisecond {
		t.Fatalf("p50 = %v, want 10ms", got)
	}
	if histogram.maximum != time.Second {
		t.Fatalf("maximum = %v, want 1s", histogram.maximum)
	}

	report := WorkloadReport{
		TotalAttempts: 41,
		Operations:    make(map[string]OperationReport),
	}
	oneCycle := expectedMix(uint64(len(workloadMix)))
	for operation, want := range map[string]uint64{
		operationCatalog:              3,
		operationCollection:           10,
		operationMatchesActionable:    4,
		operationMatchesUnfulfillable: 2,
		operationWrite:                1,
	} {
		if got := oneCycle[operation]; got != want {
			t.Fatalf("%s attempts in one cycle = %d, want %d", operation, got, want)
		}
	}
	for operation, attempts := range expectedMix(report.TotalAttempts) {
		report.Operations[operation] = OperationReport{Attempts: attempts}
	}
	if !exactMix(report) {
		t.Fatal("fixed request mix was not recognized")
	}
}

func TestCountCPUList(t *testing.T) {
	for input, want := range map[string]int{
		"0":       1,
		"0-3":     4,
		"0-1,4-5": 4,
		"broken":  0,
	} {
		if got := countCPUList(input); got != want {
			t.Errorf("countCPUList(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestRecordedV1EvidenceIsHistorical(t *testing.T) {
	raw, err := os.ReadFile(
		"../../testdata/qualification/2026-07-30-paced-systemd-pass.json",
	)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if report.FixtureVersion != "qualification-v1" ||
		!report.Gates.FormalReleaseGatePassed {
		t.Fatalf("recorded legacy evidence changed unexpectedly: %+v", report)
	}
	if report.Configuration.MemberThinkTime != FullMemberThinkTime.String() {
		t.Fatalf(
			"recorded member think time = %q, want %q",
			report.Configuration.MemberThinkTime,
			FullMemberThinkTime,
		)
	}
	current := evaluateGates(
		FullConfig(filepath.Join(t.TempDir(), "current.sqlite")),
		report,
	)
	if current.PossibleSwapJourneysPassed ||
		current.FunctionalThresholdsPassed ||
		current.FormalReleaseGatePassed {
		t.Fatalf("v1 evidence satisfied the current v3 gate: %+v", current)
	}
}
