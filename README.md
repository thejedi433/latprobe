# latprobe

Per-phase HTTP latency profiler. Answers the question "why is this API slow?"
by splitting each request into **DNS → connect → TLS → TTFB → total** and
reporting percentiles across samples.

Pure Go, stdlib only (`net/http/httptrace`), no dependencies. Cross-platform:
Linux, macOS, Windows.

## Install

```sh
go install github.com/thejedi433/latprobe/cmd/latprobe@latest
```

## Usage

```sh
latprobe https://api.example.com/v1/health
latprobe -n 30 -i 200ms https://one.example https://two.example   # compare targets
latprobe -k https://cdn.example.com                               # keep-alive: first sample cold, rest hot
```

```
..........
https://api.example.com/v1/health  (10 samples)
phase      count      min     mean      p50      p95      p99      max
dns            3    8.2ms   12.1ms   11.4ms   16.3ms   16.3ms   16.3ms
connect        3   21.0ms   23.5ms   22.8ms   27.4ms   27.4ms   27.4ms
tls            3   40.1ms   44.9ms   43.2ms   52.8ms   52.8ms   52.8ms
ttfb          10   55.0ms   61.3ms   59.8ms   78.1ms   80.2ms   80.2ms
total         10   58.4ms   66.0ms   63.1ms   85.7ms   90.3ms   90.3ms
```

Reading the table: if `dns`/`connect`/`tls` counts are lower than `ttfb`,
connection reuse kicked in (use `-k` deliberately, or without `-k` every
sample is cold). When the server is the problem, `ttfb` dominates and the
setup phases stay small.

## Flags

| flag | default | meaning |
|------|---------|---------|
| `-n` | 10 | samples per URL |
| `-i` | 500ms | delay between samples |
| `-t` | 10s | per-request timeout |
| `-k` | off | keep connections alive across samples |
| `-insecure` | off | skip TLS verification (lab use only) |
| `-q` | off | quiet: no progress dots |

Multiple URLs are probed concurrently and each gets its own summary table.
Percentiles use the nearest-rank method; zero-valued setup phases are
excluded from their own summaries. HTTP 4xx/5xx responses are recorded as
sample errors (a failing probe still tells you the latency it cost).

## Development

```sh
go test ./...          # unit tests (httptest servers, no network needed)
go vet ./...
gofmt -l .
```

## Why

curl's `-w` gives you per-request phase timings but no aggregation, and
`vegeta`/`hey` give you throughput stats but hide where the time goes.
`latprobe` sits in between: percentile latency *per connection phase*,
so you can tell a slow DNS resolver from a slow TLS stack from a slow
backend in one command.
