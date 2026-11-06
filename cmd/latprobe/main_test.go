package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thejedi433/latprobe/internal/probe"
)

func TestRunSummary(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	tmp := filepath.Join(t.TempDir(), "out.txt")
	out, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	old := os.Stdout
	os.Stdout = out
	err = run([]string{"-n", "3", "-i", "1ms", "-q", ts.URL}, out)
	os.Stdout = old
	_ = out.Sync()
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, _ := os.ReadFile(tmp)
	got := string(data)
	// localhost URLs resolve without a DNS lookup, so the dns phase row is
	// legitimately skipped; connect/ttfb/total must be present.
	for _, want := range []string{"connect", "total", "ttfb", "3 samples", ts.URL} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "dns ") {
		t.Errorf("dns row should be absent for localhost:\n%s", got)
	}
	if strings.Contains(got, "error") {
		t.Errorf("output contains error line:\n%s", got)
	}
}

func TestRunRequiresURL(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "out.txt")
	out, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := run([]string{}, out); err == nil {
		t.Fatal("run with no URLs: want error")
	}
}

func TestRunRejectsBadN(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "out.txt")
	out, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := run([]string{"-n", "0", "http://example.com"}, out); err == nil {
		t.Fatal("run with -n 0: want error")
	}
}

var (
	testCtxOnce sync.Once
	testCancel  context.CancelFunc
	testBG      context.Context
)

func NewForTest() *probe.Client { return probe.New(probe.Options{Timeout: 5 * time.Second}) }

func testCtx() context.Context {
	testCtxOnce.Do(func() { testBG, testCancel = context.WithCancel(context.Background()) })
	return testBG
}

func cancelTest() {
	testCtxOnce.Do(func() { testBG, testCancel = context.WithCancel(context.Background()) })
	if testCancel != nil {
		testCancel()
	}
}

func testMu() *sync.Mutex { return &sync.Mutex{} }

func TestProbeURLStopsOnCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer ts.Close()

	tmp := filepath.Join(t.TempDir(), "out.txt")
	out, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	c := NewForTest()
	done := make(chan int)
	go func() {
		s := probeURL(testCtx(), c, ts.URL, 1000, 50*time.Millisecond, true, out, testMu())
		done <- s.Count()
	}()
	time.Sleep(300 * time.Millisecond)
	cancelTest()
	n := <-done
	if n >= 1000 {
		t.Fatalf("probeURL ignored cancellation: got %d samples", n)
	}
	if n == 0 {
		t.Fatal("probeURL collected nothing before cancellation")
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "out.txt")
	out, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := run([]string{"-nope", "http://example.com"}, out); err == nil {
		t.Fatal("run with unknown flag: want error")
	}
}

func TestProbeURLPrintsProgressAndErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	tmp := filepath.Join(t.TempDir(), "out.txt")
	out, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	c := NewForTest()
	// Live server, quiet=false: one progress dot per sample.
	probeURL(context.Background(), c, ts.URL, 2, time.Millisecond, false, out, testMu())
	// Dead server: non-quiet path must print '!' and the sample error line.
	ts.Close()
	probeURL(context.Background(), c, ts.URL, 1, time.Millisecond, false, out, testMu())
	_ = out.Sync()

	data, _ := os.ReadFile(tmp)
	got := string(data)
	if !strings.Contains(got, "..") {
		t.Errorf("want two progress dots for the live samples, got:\n%s", got)
	}
	if !strings.Contains(got, "!") {
		t.Errorf("want '!' marker for the failed sample, got:\n%s", got)
	}
	if !strings.Contains(got, "sample 1 error:") {
		t.Errorf("want sample error line, got:\n%s", got)
	}
}

func TestProbeURLStopsBeforeFirstSampleWhenContextCanceled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tmp := filepath.Join(t.TempDir(), "out.txt")
	out, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	// A dead transport URL proves Do was never called: a call would print
	// '!' and an error line. Canceled ctx must short-circuit the loop first.
	s := probeURL(ctx, NewForTest(), ts.URL, 5, 0, false, out, testMu())
	if s.Count() != 0 {
		t.Fatalf("canceled context: samples = %d, want 0", s.Count())
	}
	_ = out.Sync()
	data, _ := os.ReadFile(tmp)
	if strings.ContainsAny(string(data), "!") {
		t.Errorf("canceled context: want zero requests, output:\n%s", data)
	}
}
