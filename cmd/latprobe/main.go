// Command latprobe measures per-phase HTTP request latency (DNS, connect,
// TLS, TTFB, total) with percentile summaries. Useful for spotting whether
// slowness lives in name resolution, connection setup, or the server itself.
//
// Usage:
//
//	latprobe [-n samples] [-i interval] [-k] [-insecure] URL [URL...]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/thejedi433/latprobe/internal/probe"
	"github.com/thejedi433/latprobe/internal/stats"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "latprobe:", err)
		os.Exit(1)
	}
}

func run(args []string, out *os.File) error {
	fs := flag.NewFlagSet("latprobe", flag.ContinueOnError)
	fs.SetOutput(out)
	n := fs.Int("n", 10, "number of samples per URL")
	interval := fs.Duration("i", 500*time.Millisecond, "delay between samples")
	timeout := fs.Duration("t", 10*time.Second, "per-request timeout")
	keepAlive := fs.Bool("k", false, "reuse connections (measures cold+hot mix)")
	insecure := fs.Bool("insecure", false, "skip TLS certificate verification")
	quiet := fs.Bool("q", false, "suppress per-sample progress dots")
	if err := fs.Parse(args); err != nil {
		return err
	}
	urls := fs.Args()
	if len(urls) == 0 {
		return fmt.Errorf("at least one URL required (see -h)")
	}
	if *n < 1 {
		return fmt.Errorf("-n must be >= 1")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opt := probe.Options{
		Timeout:         *timeout,
		Insecure:        *insecure,
		KeepAlive:       *keepAlive,
		HTTP2:           true,
		HeaderUserAgent: "latprobe/" + version,
	}
	client := probe.New(opt)

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			series := probeURL(ctx, client, url, *n, *interval, *quiet, out, &mu)
			mu.Lock()
			defer mu.Unlock()
			fmt.Fprintf(out, "\n%s  (%d samples)\n", url, series.Count())
			printSummary(out, series)
		}(u)
	}
	wg.Wait()
	return nil
}

func probeURL(ctx context.Context, c *probe.Client, url string, n int, interval time.Duration, quiet bool, out *os.File, mu *sync.Mutex) *stats.Series {
	s := &stats.Series{}
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			break
		}
		smp, err := c.Do(ctx, url)
		mu.Lock()
		if !quiet {
			if err != nil {
				fmt.Fprintf(out, "!")
			} else {
				fmt.Fprintf(out, ".")
			}
		}
		mu.Unlock()
		if err == nil {
			s.Add(smp)
		} else if ctx.Err() == nil {
			mu.Lock()
			fmt.Fprintf(out, "\n  sample %d error: %v\n", i+1, err)
			mu.Unlock()
		}
		if i < n-1 {
			select {
			case <-ctx.Done():
			case <-time.After(interval):
			}
		}
	}
	return s
}

func printSummary(out *os.File, s *stats.Series) {
	fmt.Fprintf(out, "%-8s %7s %8s %8s %8s %8s %8s %8s\n",
		"phase", "count", "min", "mean", "p50", "p95", "p99", "max")
	for _, ph := range stats.Phases {
		su := s.Summarize(ph)
		if su.Count == 0 {
			continue
		}
		fmt.Fprintf(out, "%-8s %7d %8s %8s %8s %8s %8s %8s\n",
			ph, su.Count,
			stats.Format(su.Min), stats.Format(su.Mean), stats.Format(su.P50),
			stats.Format(su.P95), stats.Format(su.P99), stats.Format(su.Max))
	}
}

const version = "0.1.0"
