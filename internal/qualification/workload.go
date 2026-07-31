package qualification

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/repository"
	"github.com/ooshdaq/ooshdaq/internal/repository/sqliterepo"
)

type operationAccumulator struct {
	attempts              uint64
	successes             uint64
	unexpectedErrors      uint64
	busyErrors            uint64
	deadlineCancellations uint64
	limitViolations       uint64
	latency               latencyHistogram
}

func newOperationAccumulator() operationAccumulator {
	return operationAccumulator{latency: newLatencyHistogram()}
}

func (accumulator *operationAccumulator) merge(other operationAccumulator) {
	accumulator.attempts += other.attempts
	accumulator.successes += other.successes
	accumulator.unexpectedErrors += other.unexpectedErrors
	accumulator.busyErrors += other.busyErrors
	accumulator.deadlineCancellations += other.deadlineCancellations
	accumulator.limitViolations += other.limitViolations
	accumulator.latency.merge(other.latency)
}

type workerResult struct {
	operations   map[string]operationAccumulator
	errorSamples []string
}

func runWorkload(
	parent context.Context,
	store *sqliterepo.Store,
	fixture fixture,
	config Config,
) WorkloadReport {
	runContext, cancel := context.WithTimeout(parent, config.Duration)
	defer cancel()
	start := make(chan struct{})
	results := make(chan workerResult, config.Workers)
	var sequence atomic.Uint64
	var workers sync.WaitGroup
	workers.Add(config.Workers)
	for workerIndex := range config.Workers {
		go func() {
			defer workers.Done()
			<-start
			results <- runWorker(
				runContext,
				store,
				fixture,
				config,
				workerIndex,
				&sequence,
			)
		}()
	}
	started := time.Now()
	close(start)
	workers.Wait()
	elapsed := time.Since(started)
	close(results)

	combined := map[string]operationAccumulator{
		operationCatalog:              newOperationAccumulator(),
		operationCollection:           newOperationAccumulator(),
		operationMatchesActionable:    newOperationAccumulator(),
		operationMatchesUnfulfillable: newOperationAccumulator(),
		operationWrite:                newOperationAccumulator(),
	}
	var errorSamples []string
	for result := range results {
		for operation, accumulator := range result.operations {
			current := combined[operation]
			current.merge(accumulator)
			combined[operation] = current
		}
		remaining := 10 - len(errorSamples)
		if remaining > 0 {
			if remaining > len(result.errorSamples) {
				remaining = len(result.errorSamples)
			}
			errorSamples = append(errorSamples, result.errorSamples[:remaining]...)
		}
	}

	report := WorkloadReport{
		RequestedDuration:   config.Duration.String(),
		Elapsed:             elapsed.String(),
		ElapsedMilliseconds: durationMilliseconds(elapsed),
		Workers:             config.Workers,
		Operations:          make(map[string]OperationReport, len(combined)),
		ErrorSamples:        errorSamples,
	}
	for _, operation := range []string{
		operationCatalog,
		operationCollection,
		operationMatchesActionable,
		operationMatchesUnfulfillable,
		operationWrite,
	} {
		accumulator := combined[operation]
		report.TotalAttempts += accumulator.attempts
		report.TotalSuccesses += accumulator.successes
		report.UnexpectedErrors += accumulator.unexpectedErrors
		report.BusyErrors += accumulator.busyErrors
		report.DeadlineCancellations += accumulator.deadlineCancellations
		report.LimitViolations += accumulator.limitViolations
		report.Operations[operation] = OperationReport{
			Attempts:              accumulator.attempts,
			Successes:             accumulator.successes,
			UnexpectedErrors:      accumulator.unexpectedErrors,
			BusyErrors:            accumulator.busyErrors,
			DeadlineCancellations: accumulator.deadlineCancellations,
			LimitViolations:       accumulator.limitViolations,
			P50Milliseconds: durationMilliseconds(
				accumulator.latency.percentile(50, 100),
			),
			P95Milliseconds: durationMilliseconds(
				accumulator.latency.percentile(95, 100),
			),
			MaxMilliseconds: durationMilliseconds(accumulator.latency.maximum),
		}
	}
	for operation, result := range report.Operations {
		if report.TotalAttempts > 0 {
			result.MixPercent = float64(result.Attempts) * 100 / float64(report.TotalAttempts)
		}
		if elapsed > 0 {
			result.AttemptsPerSecond = float64(result.Attempts) / elapsed.Seconds()
			result.SuccessesPerSecond = float64(result.Successes) / elapsed.Seconds()
		}
		report.Operations[operation] = result
	}
	if elapsed > 0 {
		report.AttemptsPerSecond = float64(report.TotalAttempts) / elapsed.Seconds()
		report.SuccessesPerSecond = float64(report.TotalSuccesses) / elapsed.Seconds()
	}
	return report
}

func runWorker(
	ctx context.Context,
	store *sqliterepo.Store,
	fixture fixture,
	config Config,
	workerIndex int,
	sequence *atomic.Uint64,
) workerResult {
	random := rand.New(rand.NewSource(
		config.Seed + int64(workerIndex+1)*1_000_003,
	))
	result := workerResult{operations: map[string]operationAccumulator{
		operationCatalog:              newOperationAccumulator(),
		operationCollection:           newOperationAccumulator(),
		operationMatchesActionable:    newOperationAccumulator(),
		operationMatchesUnfulfillable: newOperationAccumulator(),
		operationWrite:                newOperationAccumulator(),
	}}
	writeCount := 0
	for {
		select {
		case <-ctx.Done():
			return result
		default:
		}

		mixIndex := sequence.Add(1) - 1
		operation := workloadMix[mixIndex%uint64(len(workloadMix))]
		accumulator := result.operations[operation]
		accumulator.attempts++
		started := time.Now()
		err := executeOperation(
			ctx,
			store,
			fixture,
			config,
			workerIndex,
			writeCount,
			random,
			operation,
		)
		elapsed := time.Since(started)
		if operation == operationWrite {
			writeCount++
		}
		switch {
		case err == nil:
			accumulator.successes++
			accumulator.latency.observe(elapsed)
		case ctx.Err() != nil &&
			(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)):
			accumulator.deadlineCancellations++
		default:
			accumulator.unexpectedErrors++
			if errors.Is(err, errResponseLimit) {
				accumulator.limitViolations++
			}
			if repository.Classify(err) == repository.FailureBusy {
				accumulator.busyErrors++
			}
			if len(result.errorSamples) < 3 {
				result.errorSamples = append(
					result.errorSamples,
					fmt.Sprintf(
						"%s: class=%s: %v",
						operation,
						repository.Classify(err),
						err,
					),
				)
			}
		}
		result.operations[operation] = accumulator
		if !waitForThinkTime(ctx, config.MemberThinkTime) {
			return result
		}
	}
}

func waitForThinkTime(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
