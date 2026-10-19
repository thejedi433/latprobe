// Package probe measures per-phase HTTP latency with net/http/httptrace.
package probe

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/thejedi433/latprobe/internal/stats"
)

// Options configures a probing client.
type Options struct {
	Timeout         time.Duration // per-request total timeout
	Insecure        bool          // skip TLS verification
	KeepAlive       bool          // reuse connections (skips dns/connect/tls after first)
	HTTP2           bool          // enable HTTP/2
	HeaderUserAgent string
}

// Client probes one URL repeatedly.
type Client struct {
	hc  *http.Client
	opt Options
}

// New builds a Client from options.
func New(opt Options) *Client {
	if opt.Timeout <= 0 {
		opt.Timeout = 10 * time.Second
	}
	dialer := &net.Dialer{Timeout: opt.Timeout, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: opt.Timeout,
		DisableKeepAlives:   !opt.KeepAlive,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: opt.Insecure},
	}
	if !opt.HTTP2 {
		tr.ForceAttemptHTTP2 = false
		tr.TLSClientConfig.NextProtos = []string{"http/1.1"}
	} else {
		tr.ForceAttemptHTTP2 = true
	}
	return &Client{
		hc:  &http.Client{Transport: tr, Timeout: opt.Timeout},
		opt: opt,
	}
}

// Do performs one request against url and returns the phase sample.
// The response body is fully read (compressed transfer completion matters
// for total) then discarded.
func (c *Client) Do(ctx context.Context, url string) (stats.Sample, error) {
	var smp stats.Sample

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return smp, err
	}
	if c.opt.HeaderUserAgent != "" {
		req.Header.Set("User-Agent", c.opt.HeaderUserAgent)
	}

	var start, dnsStart, dnsDone, connStart, connDone, tlsDone, firstByte time.Time
	trace := &httptrace.ClientTrace{
		DNSStart:     func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:      func(httptrace.DNSDoneInfo) { dnsDone = time.Now() },
		ConnectStart: func(_, _ string) { connStart = time.Now() },
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				connDone = time.Now()
			}
		},
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tlsDone = time.Now() },
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	start = time.Now()
	resp, err := c.hc.Do(req)
	if err != nil {
		return smp, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	end := time.Now()

	if resp.StatusCode >= 400 {
		return smp, errors.New("http status " + resp.Status)
	}

	smp.Total = end.Sub(start)
	if !firstByte.IsZero() {
		smp.TTFB = firstByte.Sub(start)
	}
	if !dnsStart.IsZero() && !dnsDone.IsZero() {
		smp.DNS = dnsDone.Sub(dnsStart)
	}
	if !connStart.IsZero() && !connDone.IsZero() {
		smp.Connect = connDone.Sub(connStart)
	}
	if !connDone.IsZero() && !tlsDone.IsZero() {
		smp.TLS = tlsDone.Sub(connDone)
	}
	return smp, nil
}
