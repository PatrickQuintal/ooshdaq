package qualification

// Every complete twenty-operation cycle is exactly 15/50/20/10/5. The two
// match feeds retain their combined 30% share. The sequence is interleaved so
// even a very short smoke run exercises all operations.
var workloadMix = [...]string{
	operationCollection,
	operationMatchesActionable,
	operationCatalog,
	operationCollection,
	operationWrite,
	operationMatchesUnfulfillable,
	operationCollection,
	operationCatalog,
	operationCollection,
	operationMatchesActionable,
	operationCollection,
	operationCatalog,
	operationMatchesActionable,
	operationCollection,
	operationCollection,
	operationMatchesUnfulfillable,
	operationCollection,
	operationMatchesActionable,
	operationCollection,
	operationCollection,
}

func expectedMix(attempts uint64) map[string]uint64 {
	expected := map[string]uint64{
		operationCatalog:              0,
		operationCollection:           0,
		operationMatchesActionable:    0,
		operationMatchesUnfulfillable: 0,
		operationWrite:                0,
	}
	completeCycles := attempts / uint64(len(workloadMix))
	for _, operation := range workloadMix {
		expected[operation] += completeCycles
	}
	for index := uint64(0); index < attempts%uint64(len(workloadMix)); index++ {
		expected[workloadMix[index]]++
	}
	return expected
}

func exactMix(report WorkloadReport) bool {
	expected := expectedMix(report.TotalAttempts)
	for operation, count := range expected {
		if report.Operations[operation].Attempts != count {
			return false
		}
	}
	return true
}
