package qualification

import (
	"encoding/json"
	"time"
)

const (
	operationCatalog              = "catalog"
	operationCollection           = "collection"
	operationMatchesActionable    = "matches_actionable"
	operationMatchesUnfulfillable = "matches_unfulfillable"
	operationWrite                = "write"
)

// Report is the machine-readable evidence emitted by one qualification run.
type Report struct {
	FixtureVersion string              `json:"fixture_version"`
	GeneratedAt    time.Time           `json:"generated_at"`
	Configuration  ConfigurationReport `json:"configuration"`
	Fixture        FixtureReport       `json:"fixture"`
	Workload       WorkloadReport      `json:"workload"`
	Storage        StorageReport       `json:"storage"`
	Verification   VerificationReport  `json:"verification"`
	Environment    EnvironmentReport   `json:"environment"`
	Gates          GateReport          `json:"gates"`
}

type ConfigurationReport struct {
	DatabasePath      string `json:"database_path"`
	Seed              int64  `json:"seed"`
	RequestedUsers    int    `json:"requested_users"`
	RequestedBrands   int    `json:"requested_brands"`
	RequestedCatalogs int    `json:"requested_catalogs"`
	ItemsPerCatalog   int    `json:"items_per_catalog"`
	RequestedRows     int    `json:"requested_collection_rows"`
	Workers           int    `json:"workers"`
	Duration          string `json:"duration"`
	MemberThinkTime   string `json:"member_think_time"`
	MaxOpenConns      int    `json:"max_open_connections"`
	BusyTimeout       string `json:"busy_timeout"`
}

type FixtureReport struct {
	SHA256                       string  `json:"sha256"`
	SeedDuration                 string  `json:"seed_duration"`
	SeedDurationMilliseconds     float64 `json:"seed_duration_milliseconds"`
	BrandCount                   int     `json:"brand_count"`
	CatalogCount                 int     `json:"catalog_count"`
	ItemCount                    int     `json:"item_count"`
	UserCount                    int     `json:"user_count"`
	CollectionRowCount           int     `json:"collection_row_count"`
	RowsPerUser                  int     `json:"rows_per_user"`
	CollectionDensityPercent     float64 `json:"collection_density_percent"`
	SameCatalogReciprocalPairs   int     `json:"same_catalog_reciprocal_pairs"`
	CrossCatalogReciprocalPairs  int     `json:"cross_catalog_reciprocal_pairs"`
	SameCatalogJourneyVerified   bool    `json:"same_catalog_journey_verified"`
	CrossCatalogJourneyVerified  bool    `json:"cross_catalog_journey_verified"`
	OneWayJourneyVerified        bool    `json:"one_way_journey_verified"`
	UnfulfillableJourneyVerified bool    `json:"unfulfillable_journey_verified"`
}

type WorkloadReport struct {
	RequestedDuration     string                     `json:"requested_duration"`
	Elapsed               string                     `json:"elapsed"`
	ElapsedMilliseconds   float64                    `json:"elapsed_milliseconds"`
	Workers               int                        `json:"workers"`
	Operations            map[string]OperationReport `json:"operations"`
	TotalAttempts         uint64                     `json:"total_attempts"`
	TotalSuccesses        uint64                     `json:"total_successes"`
	AttemptsPerSecond     float64                    `json:"attempts_per_second"`
	SuccessesPerSecond    float64                    `json:"successes_per_second"`
	UnexpectedErrors      uint64                     `json:"unexpected_errors"`
	BusyErrors            uint64                     `json:"busy_errors"`
	DeadlineCancellations uint64                     `json:"deadline_cancellations"`
	LimitViolations       uint64                     `json:"limit_violations"`
	ErrorSamples          []string                   `json:"error_samples,omitempty"`
}

type OperationReport struct {
	Attempts              uint64  `json:"attempts"`
	Successes             uint64  `json:"successes"`
	UnexpectedErrors      uint64  `json:"unexpected_errors"`
	BusyErrors            uint64  `json:"busy_errors"`
	DeadlineCancellations uint64  `json:"deadline_cancellations"`
	LimitViolations       uint64  `json:"limit_violations"`
	MixPercent            float64 `json:"mix_percent"`
	AttemptsPerSecond     float64 `json:"attempts_per_second"`
	SuccessesPerSecond    float64 `json:"successes_per_second"`
	P50Milliseconds       float64 `json:"p50_milliseconds"`
	P95Milliseconds       float64 `json:"p95_milliseconds"`
	MaxMilliseconds       float64 `json:"max_milliseconds"`
}

type StorageReport struct {
	BeforeCheckpoint   FileSizeReport   `json:"before_checkpoint"`
	AfterCheckpoint    FileSizeReport   `json:"after_checkpoint"`
	PassiveCheckpoint  CheckpointResult `json:"passive_checkpoint"`
	TruncateCheckpoint CheckpointResult `json:"truncate_checkpoint"`
}

type FileSizeReport struct {
	DatabaseBytes int64 `json:"database_bytes"`
	WALBytes      int64 `json:"wal_bytes"`
	SHMBytes      int64 `json:"shm_bytes"`
}

type CheckpointResult struct {
	Busy                 int     `json:"busy"`
	LogFrames            int     `json:"log_frames"`
	CheckpointedFrames   int     `json:"checkpointed_frames"`
	Duration             string  `json:"duration"`
	DurationMilliseconds float64 `json:"duration_milliseconds"`
}

type VerificationReport struct {
	ReadinessPassed   bool     `json:"readiness_passed"`
	ForeignKeysPassed bool     `json:"foreign_keys_passed"`
	IntegrityPassed   bool     `json:"integrity_passed"`
	QueryPlanBounded  bool     `json:"query_plan_bounded"`
	QueryPlanDetails  []string `json:"query_plan_details"`
	RequiredIndexes   []string `json:"required_indexes"`
}

type EnvironmentReport struct {
	GOOS                   string  `json:"goos"`
	GOARCH                 string  `json:"goarch"`
	GoVersion              string  `json:"go_version"`
	ExpectedGoVersion      string  `json:"expected_go_version"`
	GOMAXPROCS             int     `json:"gomaxprocs"`
	AllowedCPUs            int     `json:"allowed_cpus"`
	CgroupCPUQuota         float64 `json:"cgroup_cpu_quota,omitempty"`
	CgroupMemoryLimitBytes int64   `json:"cgroup_memory_limit_bytes,omitempty"`
	GoMemoryLimitBytes     int64   `json:"go_memory_limit_bytes,omitempty"`
	BaselineRSSBytes       int64   `json:"baseline_rss_bytes,omitempty"`
	FinalRSSBytes          int64   `json:"final_rss_bytes,omitempty"`
	PeakRSSBytes           int64   `json:"peak_rss_bytes,omitempty"`
	HeapAllocBytes         uint64  `json:"heap_alloc_bytes"`
	GoRuntimeSysBytes      uint64  `json:"go_runtime_sys_bytes"`
	HardCPUConstraint      bool    `json:"hard_cpu_constraint"`
	HardMemoryConstraint   bool    `json:"hard_memory_constraint"`
}

type GateReport struct {
	FixedReleaseShape          bool `json:"fixed_release_shape"`
	FixedReleaseWorkload       bool `json:"fixed_release_workload"`
	ExactRequestMix            bool `json:"exact_request_mix"`
	ZeroUnexpectedErrors       bool `json:"zero_unexpected_errors"`
	NoBusyErrors               bool `json:"no_busy_errors"`
	LatencyThresholdsPassed    bool `json:"latency_thresholds_passed"`
	ResponseBoundsPassed       bool `json:"response_bounds_passed"`
	ReciprocalJourneysPassed   bool `json:"reciprocal_journeys_passed"`
	PossibleSwapJourneysPassed bool `json:"possible_swap_journeys_passed"`
	QueryPlanPassed            bool `json:"query_plan_passed"`
	ReadinessIntegrityPassed   bool `json:"readiness_integrity_passed"`
	CheckpointPassed           bool `json:"checkpoint_passed"`
	SingleCPUExecution         bool `json:"single_cpu_execution"`
	ObservedMemoryWithin256MiB bool `json:"observed_memory_within_256_mib"`
	HardResourceLimitsVerified bool `json:"hard_resource_limits_verified"`
	PinnedGoVersionVerified    bool `json:"pinned_go_version_verified"`
	FunctionalThresholdsPassed bool `json:"functional_thresholds_passed"`
	FormalReleaseGatePassed    bool `json:"formal_release_gate_passed"`
}

// JSON renders a stable, indented evidence document.
func (report Report) JSON() ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
