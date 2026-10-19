package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDoAgainstTestServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte("hello"))
	}))
	defer ts.Close()

	c := New(Options{Timeout: 5 * time.Second})
	smp, err := c.Do(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if smp.Total <= 0 {
		t.Fatalf("Total = %v, want > 0", smp.Total)
	}
	if smp.TTFB <= 0 || smp.TTFB > smp.Total {
		t.Fatalf("TTFB = %v not within (0, total=%v]", smp.TTFB, smp.Total)
	}
	// http:// (not https) must not report TLS time.
	if smp.TLS != 0 {
		t.Fatalf("TLS = %v on plain HTTP, want 0", smp.TLS)
	}
	// Connect to 127.0.0.1 goes through no resolver path we can guarantee,
	// but connect must be recorded at least once.
	if smp.Connect == 0 {
		t.Fatalf("Connect = 0, want > 0")
	}
}

func TestDoErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := New(Options{Timeout: 5 * time.Second})
	if _, err := c.Do(context.Background(), ts.URL); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("Do on 500: err = %v, want status error", err)
	}
}

func TestDoBadURL(t *testing.T) {
	c := New(Options{Timeout: time.Second})
	if _, err := c.Do(context.Background(), "://nope"); err == nil {
		t.Fatal("Do on malformed URL: want error")
	}
}

func TestDoContextCanceled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()

	c := New(Options{Timeout: 10 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := c.Do(ctx, ts.URL); err == nil {
		t.Fatal("Do with canceled ctx: want error")
	}
}

func TestUserAgentHeader(t *testing.T) {
	var got string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
	}))
	defer ts.Close()

	c := New(Options{Timeout: 5 * time.Second, HeaderUserAgent: "probe-test"})
	if _, err := c.Do(context.Background(), ts.URL); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got != "probe-test" {
		t.Fatalf("User-Agent = %q, want probe-test", got)
	}
}

func TestNewDefaultsTimeout(t *testing.T) {
	c := New(Options{})
	if c.opt.Timeout != 10*time.Second {
		t.Fatalf("default timeout = %v, want 10s", c.opt.Timeout)
	}
}

func TestDoTLSAndInsecure(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secure"))
	}))
	defer ts.Close()

	c := New(Options{Timeout: 5 * time.Second, Insecure: true, HTTP2: true})
	smp, err := c.Do(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Do TLS: %v", err)
	}
	if smp.TLS <= 0 {
		t.Fatalf("TLS = %v on https, want > 0", smp.TLS)
	}
	if smp.Total <= smp.TLS {
		t.Fatalf("Total (%v) should exceed TLS (%v)", smp.Total, smp.TLS)
	}
}

func TestDoTLSCertRejected(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ts.Close()

	c := New(Options{Timeout: 5 * time.Second, Insecure: false})
	if _, err := c.Do(context.Background(), ts.URL); err == nil {
		t.Fatal("Do with self-signed cert and Insecure=false: want error")
	}
}
