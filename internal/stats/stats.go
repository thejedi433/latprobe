// Package stats computes latency percentiles over samples of phase timings.
package stats

import (
	"fmt"
	"sort"
	"time"
)

// Phases names the latency stages measured for one request.
var Phases = []string{"dns", "connect", "tls", "ttfb", "total"}

// Sample holds the per-phase durations of one request. Zero means the
// phase did not occur (e.g. connection reuse skips dns/connect/tls).
type Sample struct {
	DNS     time.Duration
	Connect time.Duration
	TLS     time.Duration
	TTFB    time.Duration
	Total   time.Duration
}

// Series accumulates samples for one target.
type Series struct {
	Samples []Sample
}

// Add records a sample.
func (s *Series) Add(smp Sample) { s.Samples = append(s.Samples, smp) }

// Count returns the number of samples.
func (s *Series) Count() int { return len(s.Samples) }

// Phase returns the slice of durations for a named phase across samples.
func (s *Series) Phase(name string) []time.Duration {
	out := make([]time.Duration, 0, len(s.Samples))
	for _, sm := range s.Samples {
		var d time.Duration
		switch name {
		case "dns":
			d = sm.DNS
		case "connect":
			d = sm.Connect
		case "tls":
			d = sm.TLS
		case "ttfb":
			d = sm.TTFB
		case "total":
			d = sm.Total
		default:
			continue
		}
		out = append(out, d)
	}
	return out
}

// Summary is the aggregate of one phase.
type Summary struct {
	Count int
	Min   time.Duration
	Mean  time.Duration
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	Max   time.Duration
}

// Summarize computes min/mean/p50/p95/p99/max for a phase. Measured-only:
// zero values (phase skipped, e.g. keep-alive reuse) are excluded except
// for "total" and "ttfb" where every request has them.
func (s *Series) Summarize(name string) Summary {
	vals := s.Phase(name)
	if name != "total" && name != "ttfb" {
		filtered := vals[:0]
		for _, v := range vals {
			if v > 0 {
				filtered = append(filtered, v)
			}
		}
		vals = filtered
	}
	if len(vals) == 0 {
		return Summary{}
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
	var sum time.Duration
	for _, v := range vals {
		sum += v
	}
	return Summary{
		Count: len(vals),
		Min:   vals[0],
		Mean:  time.Duration(int64(sum) / int64(len(vals))),
		P50:   percentile(vals, 50),
		P95:   percentile(vals, 95),
		P99:   percentile(vals, 99),
		Max:   vals[len(vals)-1],
	}
}

// percentile returns the p-th nearest-rank percentile of sorted vals.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := int(((float64(len(sorted)) * p) + 99) / 100) // ceil(len*p/100)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// Format renders a duration human-style with ms precision; "–" for zero.
func Format(d time.Duration) string {
	if d == 0 {
		return "–"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
	}
	return fmt.Sprintf("%.3fs", d.Seconds())
}
