package qualification

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

type memorySnapshot struct {
	rss  int64
	peak int64
}

func collectEnvironment(baseline memorySnapshot) EnvironmentReport {
	current := readProcessMemory()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	allowedCPUs := readAllowedCPUCount()
	cgroupCPU, cgroupMemory := readCgroupLimits()
	goMemoryLimit := debug.SetMemoryLimit(-1)
	hardCPU := allowedCPUs == 1 || (cgroupCPU > 0 && cgroupCPU <= 1)
	hardMemory := cgroupMemory > 0 && cgroupMemory <= targetMemoryBytes
	return EnvironmentReport{
		GOOS:                   runtime.GOOS,
		GOARCH:                 runtime.GOARCH,
		GoVersion:              runtime.Version(),
		ExpectedGoVersion:      "go1.26.5",
		GOMAXPROCS:             runtime.GOMAXPROCS(0),
		AllowedCPUs:            allowedCPUs,
		CgroupCPUQuota:         cgroupCPU,
		CgroupMemoryLimitBytes: cgroupMemory,
		GoMemoryLimitBytes:     goMemoryLimit,
		BaselineRSSBytes:       baseline.rss,
		FinalRSSBytes:          current.rss,
		PeakRSSBytes:           max(baseline.peak, current.peak),
		HeapAllocBytes:         memory.HeapAlloc,
		GoRuntimeSysBytes:      memory.Sys,
		HardCPUConstraint:      hardCPU,
		HardMemoryConstraint:   hardMemory,
	}
}

func readProcessMemory() memorySnapshot {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return memorySnapshot{}
	}
	defer file.Close()
	var result memorySnapshot
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "VmRSS":
			result.rss = value * 1024
		case "VmHWM":
			result.peak = value * 1024
		}
	}
	return result
}

func readAllowedCPUCount() int {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return runtime.NumCPU()
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "Cpus_allowed_list:") {
			continue
		}
		return countCPUList(strings.TrimSpace(strings.TrimPrefix(
			line,
			"Cpus_allowed_list:",
		)))
	}
	return runtime.NumCPU()
}

func countCPUList(value string) int {
	total := 0
	for _, part := range strings.Split(value, ",") {
		bounds := strings.SplitN(part, "-", 2)
		first, err := strconv.Atoi(bounds[0])
		if err != nil {
			return 0
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.Atoi(bounds[1])
			if err != nil || last < first {
				return 0
			}
		}
		total += last - first + 1
	}
	return total
}

func readCgroupLimits() (float64, int64) {
	raw, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return 0, 0
	}
	var relative string
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) == 3 && fields[0] == "0" && fields[1] == "" {
			relative = fields[2]
			break
		}
	}
	if relative == "" {
		return 0, 0
	}
	root := "/sys/fs/cgroup"
	directory := filepath.Join(root, filepath.Clean("/"+relative))
	if !strings.HasPrefix(directory, root+string(filepath.Separator)) {
		return 0, 0
	}
	return readCPUQuota(filepath.Join(directory, "cpu.max")),
		readByteLimit(filepath.Join(directory, "memory.max"))
}

func readCPUQuota(path string) float64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 || fields[0] == "max" {
		return 0
	}
	quota, quotaErr := strconv.ParseFloat(fields[0], 64)
	period, periodErr := strconv.ParseFloat(fields[1], 64)
	if quotaErr != nil || periodErr != nil || period <= 0 {
		return 0
	}
	return quota / period
}

func readByteLimit(path string) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "max" {
		return 0
	}
	limit, err := strconv.ParseInt(value, 10, 64)
	if err != nil || limit <= 0 {
		return 0
	}
	return limit
}
