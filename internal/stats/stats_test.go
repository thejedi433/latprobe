package stats

import (
	"testing"
	"time"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestSeriesPhase(t *testing.T) {
	s := &Series{}
	s.Add(Sample{DNS: ms(10), Connect: ms(20), TLS: ms(30), TTFB: ms(50), Total: ms(100)})
	s.Add(Sample{DNS: 0, Connect: 0, TLS: 0, TTFB: ms(40), Total: ms(60)})

	if got := s.Count(); got != 2 {
		t.Fatalf("Count = %d, want 2", got)
	}
	if got := s.Phase("dns"); len(got) != 2 || got[0] != ms(10) {
		t.Fatalf("Phase(dns) = %v", got)
	}
	if got := s.Phase("bogus"); len(got) != 0 {
		t.Fatalf("Phase(bogus) = %v, want empty", got)
	}
}

func TestSummarizeFiltersZeroPhases(t *testing.T) {
	s := &Series{}
	s.Add(Sample{DNS: ms(10), TTFB: ms(50), Total: ms(100)})
	s.Add(Sample{DNS: 0, TTFB: ms(40), Total: ms(60)})

	dns := s.Summarize("dns")
	if dns.Count != 1 {
		t.Fatalf("dns count = %d, want 1 (zero filtered)", dns.Count)
	}
	total := s.Summarize("total")
	if total.Count != 2 {
		t.Fatalf("total count = %d, want 2 (zeros kept)", total.Count)
	}
	if total.Min != ms(60) || total.Max != ms(100) {
		t.Fatalf("total min/max = %v/%v", total.Min, total.Max)
	}
	mean := total.Mean
	if mean != ms(80) {
		t.Fatalf("total mean = %v, want 80ms", mean)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	s := &Series{}
	s.Add(Sample{TLS: 0, Total: ms(5), TTFB: ms(5)})
	if got := s.Summarize("tls"); got.Count != 0 {
		t.Fatalf("tls summary = %+v, want zero-value", got)
	}
}

func TestPercentileNearestRank(t *testing.T) {
	// 1..100 ms sorted.
	vals := make([]time.Duration, 100)
	for i := range vals {
		vals[i] = ms(i + 1)
	}
	tests := []struct {
		p    float64
		want time.Duration
	}{
		{0, ms(1)},
		{50, ms(50)},
		{95, ms(95)},
		{99, ms(99)},
		{100, ms(100)},
	}
	for _, tt := range tests {
		if got := percentile(vals, tt.p); got != tt.want {
			t.Errorf("percentile(p=%.0f) = %v, want %v", tt.p, got, tt.want)
		}
	}
	// single sample: all percentiles equal it
	one := []time.Duration{ms(42)}
	if got := percentile(one, 99); got != ms(42) {
		t.Errorf("single-sample p99 = %v, want 42ms", got)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Errorf("empty percentile = %v, want 0", got)
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "–"},
		{500 * time.Microsecond, "500µs"},
		{1500 * time.Microsecond, "1.5ms"},
		{25 * time.Millisecond, "25.0ms"},
		{1500 * time.Millisecond, "1.500s"},
	}
	for _, tt := range tests {
		if got := Format(tt.d); got != tt.want {
			t.Errorf("Format(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestPhaseAllNames(t *testing.T) {
	s := &Series{}
	s.Add(Sample{DNS: ms(1), Connect: ms(2), TLS: ms(3), TTFB: ms(4), Total: ms(5)})
	for name, want := range map[string]time.Duration{
		"dns": ms(1), "connect": ms(2), "tls": ms(3), "ttfb": ms(4), "total": ms(5),
	} {
		if got := s.Phase(name)[0]; got != want {
			t.Errorf("Phase(%s)[0] = %v, want %v", name, got, want)
		}
	}
}

func TestPercentileClampsRank(t *testing.T) {
	vals := []time.Duration{ms(1), ms(2), ms(3)}
	if got := percentile(vals, 1); got != ms(1) {
		t.Errorf("p1 = %v, want 1ms (rank ceil(0.03)=1)", got)
	}
	if got := percentile(vals, 100); got != ms(3) {
		t.Errorf("p100 = %v, want 3ms", got)
	}
	if got := percentile(vals, -5); got != ms(1) {
		t.Errorf("negative p = %v, want min", got)
	}
}
