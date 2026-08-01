package qualification

import "time"

var latencyBounds = buildLatencyBounds()

type latencyHistogram struct {
	buckets []uint64
	count   uint64
	maximum time.Duration
}

func newLatencyHistogram() latencyHistogram {
	return latencyHistogram{buckets: make([]uint64, len(latencyBounds))}
}

func (histogram *latencyHistogram) observe(duration time.Duration) {
	index := len(latencyBounds) - 1
	for candidate, bound := range latencyBounds {
		if duration <= bound {
			index = candidate
			break
		}
	}
	histogram.buckets[index]++
	histogram.count++
	if duration > histogram.maximum {
		histogram.maximum = duration
	}
}

func (histogram *latencyHistogram) merge(other latencyHistogram) {
	if histogram.buckets == nil {
		histogram.buckets = make([]uint64, len(latencyBounds))
	}
	for index, count := range other.buckets {
		histogram.buckets[index] += count
	}
	histogram.count += other.count
	if other.maximum > histogram.maximum {
		histogram.maximum = other.maximum
	}
}

func (histogram latencyHistogram) percentile(numerator, denominator uint64) time.Duration {
	if histogram.count == 0 {
		return 0
	}
	target := (histogram.count*numerator + denominator - 1) / denominator
	var observed uint64
	for index, count := range histogram.buckets {
		observed += count
		if observed >= target {
			if index == len(latencyBounds)-1 {
				return histogram.maximum
			}
			return latencyBounds[index]
		}
	}
	return histogram.maximum
}

func buildLatencyBounds() []time.Duration {
	bounds := make([]time.Duration, 0, 371)
	for value := 100 * time.Microsecond; value <= 10*time.Millisecond; value += 100 * time.Microsecond {
		bounds = append(bounds, value)
	}
	for value := 11 * time.Millisecond; value <= 100*time.Millisecond; value += time.Millisecond {
		bounds = append(bounds, value)
	}
	for value := 110 * time.Millisecond; value <= time.Second; value += 10 * time.Millisecond {
		bounds = append(bounds, value)
	}
	for value := 1100 * time.Millisecond; value <= 10*time.Second; value += 100 * time.Millisecond {
		bounds = append(bounds, value)
	}
	bounds = append(bounds, time.Duration(1<<63-1))
	return bounds
}

func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
