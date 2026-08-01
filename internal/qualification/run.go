package qualification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/repository/sqliterepo"
)

// Run creates a new SQLite qualification database, seeds deterministic
// fictional data, executes the configured workload, and returns observed
// evidence. It refuses to overwrite a database or its WAL sidecars.
func Run(ctx context.Context, config Config) (Report, error) {
	config.DatabasePath = strings.TrimSpace(config.DatabasePath)
	if err := config.validate(); err != nil {
		return Report{}, err
	}
	config.DatabasePath = filepath.Clean(config.DatabasePath)
	if err := prepareDatabasePath(config.DatabasePath); err != nil {
		return Report{}, err
	}
	report := Report{
		FixtureVersion: fixtureVersion,
		GeneratedAt:    time.Now().UTC(),
		Configuration: ConfigurationReport{
			DatabasePath:      config.DatabasePath,
			Seed:              config.Seed,
			RequestedUsers:    config.Users,
			RequestedBrands:   config.Brands,
			RequestedCatalogs: config.Catalogs,
			ItemsPerCatalog:   config.ItemsPerCatalog,
			RequestedRows:     config.CollectionRows,
			Workers:           config.Workers,
			Duration:          config.Duration.String(),
			MemberThinkTime:   config.MemberThinkTime.String(),
			MaxOpenConns:      config.MaxOpenConns,
			BusyTimeout:       config.BusyTimeout.String(),
		},
	}
	store, err := sqliterepo.Open(ctx, sqliterepo.Config{
		Path:         config.DatabasePath,
		MaxOpenConns: config.MaxOpenConns,
		BusyTimeout:  config.BusyTimeout,
	})
	if err != nil {
		return report, fmt.Errorf("open qualification store: %w", err)
	}
	defer store.Close()

	fixture, err := seedFixture(ctx, store, config)
	if err != nil {
		return report, err
	}
	report.Fixture = fixture.report
	crossPassed, samePassed, oneWayPassed, unfulfillablePassed, err :=
		verifyPossibleSwapJourneys(
			ctx,
			store,
			fixture,
		)
	if err != nil {
		return report, err
	}
	report.Fixture.CrossCatalogJourneyVerified = crossPassed
	report.Fixture.SameCatalogJourneyVerified = samePassed
	report.Fixture.OneWayJourneyVerified = oneWayPassed
	report.Fixture.UnfulfillableJourneyVerified = unfulfillablePassed

	baseline := readProcessMemory()
	report.Workload = runWorkload(ctx, store, fixture, config)
	report.Storage, report.Verification, err = collectStorageAndPlan(ctx, store, config)
	var verificationErrors []error
	if err != nil {
		verificationErrors = append(verificationErrors, err)
	}
	if pingErr := store.Ping(ctx); pingErr != nil {
		verificationErrors = append(
			verificationErrors,
			fmt.Errorf("qualification readiness: %w", pingErr),
		)
	} else {
		report.Verification.ReadinessPassed = true
	}
	if checkErr := store.Check(ctx, true); checkErr != nil {
		verificationErrors = append(
			verificationErrors,
			fmt.Errorf("qualification integrity: %w", checkErr),
		)
	} else {
		report.Verification.ForeignKeysPassed = true
		report.Verification.IntegrityPassed = true
	}
	report.Environment = collectEnvironment(baseline)
	report.Gates = evaluateGates(config, report)
	return report, errors.Join(verificationErrors...)
}

func prepareDatabasePath(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if _, err := os.Lstat(candidate); err == nil {
			return fmt.Errorf(
				"qualification refuses to overwrite existing path %q",
				candidate,
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect qualification path %q: %w", candidate, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create qualification database directory: %w", err)
	}
	return nil
}

func evaluateGates(config Config, report Report) GateReport {
	fixedShape := report.FixtureVersion == fixtureVersion &&
		config.isFixedReleaseShape() &&
		report.Fixture.UserCount == FullUserCount &&
		report.Fixture.BrandCount == FullBrandCount &&
		report.Fixture.CatalogCount == FullCatalogCount &&
		report.Fixture.ItemCount == FullCatalogCount*FullItemsPerCatalog &&
		report.Fixture.CollectionRowCount == FullCollectionRowCount
	fixedWorkload := config.isFixedReleaseWorkload() &&
		report.Workload.ElapsedMilliseconds >= durationMilliseconds(config.Duration)
	exactRequestMix := report.Workload.TotalAttempts >= uint64(len(workloadMix)) &&
		exactMix(report.Workload)
	latencyPassed := operationLatencyPassed(
		report.Workload.Operations[operationCatalog],
		time.Second,
	) && operationLatencyPassed(
		report.Workload.Operations[operationCollection],
		time.Second,
	) && operationLatencyPassed(
		report.Workload.Operations[operationMatchesActionable],
		2*time.Second,
	) && operationLatencyPassed(
		report.Workload.Operations[operationMatchesUnfulfillable],
		2*time.Second,
	) && operationLatencyPassed(
		report.Workload.Operations[operationWrite],
		time.Second,
	)
	readinessIntegrity := report.Verification.ReadinessPassed &&
		report.Verification.ForeignKeysPassed &&
		report.Verification.IntegrityPassed
	checkpointPassed := report.Storage.PassiveCheckpoint.Busy == 0 &&
		report.Storage.TruncateCheckpoint.Busy == 0 &&
		report.Storage.PassiveCheckpoint.Duration != "" &&
		report.Storage.TruncateCheckpoint.Duration != ""
	reciprocalPassed := report.Fixture.CrossCatalogJourneyVerified &&
		report.Fixture.SameCatalogJourneyVerified
	possibleSwapPassed := reciprocalPassed &&
		report.Fixture.OneWayJourneyVerified &&
		report.Fixture.UnfulfillableJourneyVerified
	singleCPU := report.Environment.GOMAXPROCS == 1 &&
		report.Environment.HardCPUConstraint
	observedMemory := report.Environment.PeakRSSBytes > 0 &&
		report.Environment.PeakRSSBytes <= targetMemoryBytes
	hardLimits := report.Environment.HardCPUConstraint &&
		report.Environment.HardMemoryConstraint
	pinnedGoVersion := report.Environment.GoVersion ==
		report.Environment.ExpectedGoVersion
	gates := GateReport{
		FixedReleaseShape:          fixedShape,
		FixedReleaseWorkload:       fixedWorkload,
		ExactRequestMix:            exactRequestMix,
		ZeroUnexpectedErrors:       report.Workload.UnexpectedErrors == 0,
		NoBusyErrors:               report.Workload.BusyErrors == 0,
		LatencyThresholdsPassed:    latencyPassed,
		ResponseBoundsPassed:       report.Workload.LimitViolations == 0,
		ReciprocalJourneysPassed:   reciprocalPassed,
		PossibleSwapJourneysPassed: possibleSwapPassed,
		QueryPlanPassed:            report.Verification.QueryPlanBounded,
		ReadinessIntegrityPassed:   readinessIntegrity,
		CheckpointPassed:           checkpointPassed,
		SingleCPUExecution:         singleCPU,
		ObservedMemoryWithin256MiB: observedMemory,
		HardResourceLimitsVerified: hardLimits,
		PinnedGoVersionVerified:    pinnedGoVersion,
	}
	gates.FunctionalThresholdsPassed =
		gates.ExactRequestMix &&
			gates.ZeroUnexpectedErrors &&
			gates.NoBusyErrors &&
			gates.LatencyThresholdsPassed &&
			gates.ResponseBoundsPassed &&
			gates.PossibleSwapJourneysPassed &&
			gates.QueryPlanPassed &&
			gates.ReadinessIntegrityPassed &&
			gates.CheckpointPassed
	gates.FormalReleaseGatePassed =
		gates.FixedReleaseShape &&
			gates.FixedReleaseWorkload &&
			gates.FunctionalThresholdsPassed &&
			gates.SingleCPUExecution &&
			gates.ObservedMemoryWithin256MiB &&
			gates.HardResourceLimitsVerified &&
			gates.PinnedGoVersionVerified
	return gates
}

func operationLatencyPassed(result OperationReport, threshold time.Duration) bool {
	return result.Successes > 0 &&
		result.P95Milliseconds < durationMilliseconds(threshold)
}
