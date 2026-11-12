package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

var errNotReady = errors.New("service did not become ready")

type options struct {
	bin       string
	out       string
	rates     []int
	duration  time.Duration
	warmup    time.Duration
	reps      int
	streams   int
	proxyCPUs string
	mockCPUs  string
	size      int
}

type runResult struct {
	Target   string  `json:"target"`
	Rate     int     `json:"rate"`
	Rep      int     `json:"rep"`
	Requests uint64  `json:"requests"`
	Success  float64 `json:"success"`
	P50us    float64 `json:"p50_us"`
	P90us    float64 `json:"p90_us"`
	P99us    float64 `json:"p99_us"`
	P999us   float64 `json:"p999_us"`
	MaxUs    float64 `json:"max_us"`
	CPUPer1k float64 `json:"cpu_ms_per_1k,omitzero"`
}

type streamResult struct {
	Target   string  `json:"target"`
	N        int     `json:"n"`
	TTFBp50  float64 `json:"ttfb_p50_us"`
	TTFBp99  float64 `json:"ttfb_p99_us"`
	TotalP50 float64 `json:"total_p50_us"`
	TotalP99 float64 `json:"total_p99_us"`
}

type summary struct {
	Env     map[string]string `json:"env"`
	Runs    []runResult       `json:"runs"`
	Streams []streamResult    `json:"streams"`
	Payload int               `json:"payload_bytes"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseRates(s string) ([]int, error) {
	var out []int
	for f := range strings.SplitSeq(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return nil, fmt.Errorf("parse rate %q: %w", f, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func run(args []string) error {
	fs := flag.NewFlagSet("loadgen", flag.ContinueOnError)
	var o options
	rates := fs.String("rates", "500,1000,2000", "comma separated request rates per second")
	fs.StringVar(&o.bin, "bin", "../bin", "directory with maskwire and mockllm binaries")
	fs.StringVar(&o.out, "out", "results", "results directory")
	fs.DurationVar(&o.duration, "duration", 15*time.Second, "measured duration per run")
	fs.DurationVar(&o.warmup, "warmup", 3*time.Second, "discarded warm-up per run")
	fs.IntVar(&o.reps, "reps", 3, "repetitions per rate and target")
	fs.IntVar(&o.streams, "streams", 300, "sequential streaming requests per target")
	fs.StringVar(&o.proxyCPUs, "proxy-cpus", "4-7", "cpu list for maskwire")
	fs.StringVar(&o.mockCPUs, "mock-cpus", "8-11", "cpu list for mockllm")
	fs.IntVar(&o.size, "size", 4096, "prompt size in bytes")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	var err error
	if o.rates, err = parseRates(*rates); err != nil {
		return err
	}
	ctx := context.Background()
	work, err := os.MkdirTemp("", "maskwire-load-")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(work)
	stop, proxyPID, err := startStack(ctx, o, work)
	defer stop()
	if err != nil {
		return err
	}
	body := payload(o.size, false)
	s := summary{Env: environment(o), Payload: len(body)}
	for _, rate := range o.rates {
		for rep := range o.reps {
			for _, tg := range []struct{ name, url string }{
				{"direct", "http://127.0.0.1:19000/v1/chat/completions"},
				{"maskwire", "http://127.0.0.1:18090/v1/chat/completions"},
			} {
				pid := 0
				if tg.name == "maskwire" {
					pid = proxyPID
				}
				r, err := attack(tg.name, tg.url, body, rate, rep, o, pid)
				if err != nil {
					return err
				}
				fmt.Fprintf(
					os.Stderr,
					"%-8s rate=%-5d rep=%d p50=%.0fus p99=%.0fus ok=%.4f\n",
					r.Target,
					r.Rate,
					r.Rep,
					r.P50us,
					r.P99us,
					r.Success,
				)
				s.Runs = append(s.Runs, r)
			}
		}
	}
	for _, tg := range []struct{ name, url string }{
		{"direct", "http://127.0.0.1:19000/v1/chat/completions"},
		{"maskwire", "http://127.0.0.1:18090/v1/chat/completions"},
	} {
		sr, err := streamProbe(ctx, tg.name, tg.url, payload(o.size, true), o.streams)
		if err != nil {
			return err
		}
		s.Streams = append(s.Streams, sr)
	}
	return write(o.out, s)
}

func taskset(cpus, bin string, args ...string) *exec.Cmd {
	if _, err := exec.LookPath("taskset"); err == nil && cpus != "" {
		return exec.Command("taskset", append([]string{"-c", cpus, bin}, args...)...)
	}
	return exec.Command(bin, args...)
}

func startStack(ctx context.Context, o options, work string) (stop func(), proxyPID int, err error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return func() {}, 0, fmt.Errorf("key: %w", err)
	}
	cfgPath := filepath.Join(work, "maskwire.yaml")
	cfg := fmt.Sprintf(`server:
  addr: "127.0.0.1:18090"
  admin_addr: "127.0.0.1:19090"
  shutdown_drain_delay: 0s
audit:
  enabled: true
  dir: %s
log:
  level: warn
routes:
  - name: openai
    path: /v1/chat/completions
    upstream: http://127.0.0.1:19000
    codec: openai
tenants:
  - id: bench
    key_sha256: "0000000000000000000000000000000000000000000000000000000000000000"
`, filepath.Join(work, "audit"))
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		return func() {}, 0, fmt.Errorf("write config: %w", err)
	}
	gmp := "GOMAXPROCS=4"
	mock := taskset(o.mockCPUs, filepath.Join(o.bin, "mockllm"), "-addr", "127.0.0.1:19000")
	mock.Env = append(os.Environ(), gmp)
	proxy := taskset(o.proxyCPUs, filepath.Join(o.bin, "maskwire"), "serve", "--config", cfgPath)
	proxy.Env = append(
		os.Environ(),
		gmp,
		"MASKWIRE_KEYS__MASTER_KEY="+hex.EncodeToString(key),
		"MASKWIRE_SERVER__DEFAULT_TENANT=bench",
	)
	proxy.Stderr = os.Stderr
	cmds := []*exec.Cmd{mock, proxy}
	stop = func() {
		for _, c := range cmds {
			if c.Process != nil {
				_ = c.Process.Signal(os.Interrupt)
				_ = c.Wait()
			}
		}
	}
	for _, c := range cmds {
		if err := c.Start(); err != nil {
			return stop, 0, fmt.Errorf("start %s: %w", c.Path, err)
		}
	}
	for _, u := range []string{"http://127.0.0.1:19000/healthz", "http://127.0.0.1:19090/readyz"} {
		if err := waitReady(ctx, u); err != nil {
			return stop, 0, err
		}
	}
	return stop, proxy.Process.Pid, nil
}

func waitReady(ctx context.Context, url string) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return fmt.Errorf("ready request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%w: %s", errNotReady, url)
}

func payload(size int, stream bool) []byte {
	pii := "Customer Ana Souza (ana.souza@correio.pt, CPF 529.982.247-25, tel +351 912 345 678) asked about " +
		"the refund to IBAN PT50 0002 0123 1234 5678 9015 4 from host 8.8.8.8. "
	filler := "Please summarise the ticket history, keep a neutral tone, list the next actions with owners and " +
		"deadlines, and avoid promising anything the policy does not allow. "
	var sb strings.Builder
	sb.WriteString(pii)
	for sb.Len() < size {
		sb.WriteString(filler)
	}
	text := sb.String()[:size]
	b, err := json.Marshal(map[string]any{
		"model":  "gpt-4.1-mini",
		"stream": stream,
		"messages": []any{
			map[string]string{"role": "system", "content": "You are a support assistant."},
			map[string]string{"role": "user", "content": text},
		},
	})
	if err != nil {
		return nil
	}
	return b
}

func cpuTicks(pid int) (float64, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, fmt.Errorf("read stat: %w", err)
	}
	fields := strings.Fields(string(b[bytes.LastIndexByte(b, ')')+2:]))
	ut, err1 := strconv.ParseFloat(fields[11], 64)
	st, err2 := strconv.ParseFloat(fields[12], 64)
	if err := errors.Join(err1, err2); err != nil {
		return 0, fmt.Errorf("parse stat: %w", err)
	}
	return (ut + st) * 10, nil
}

func attack(name, url string, body []byte, rate, rep int, o options, pid int) (runResult, error) {
	tr := vegeta.NewStaticTargeter(vegeta.Target{
		Method: http.MethodPost, URL: url, Body: body,
		Header: http.Header{"Content-Type": []string{"application/json"}},
	})
	newAttacker := func() *vegeta.Attacker {
		return vegeta.NewAttacker(
			vegeta.Workers(64),
			vegeta.MaxWorkers(4096),
			vegeta.KeepAlive(true),
			vegeta.Timeout(10*time.Second),
		)
	}
	pace := vegeta.Rate{Freq: rate, Per: time.Second}
	warm := 0
	for range newAttacker().Attack(tr, pace, o.warmup, "warmup") {
		warm++
	}
	var before float64
	if pid > 0 {
		var err error
		if before, err = cpuTicks(pid); err != nil {
			return runResult{}, err
		}
	}
	var m vegeta.Metrics
	for res := range newAttacker().Attack(tr, pace, o.duration, name) {
		m.Add(res)
	}
	m.Close()
	r := runResult{
		Target: name, Rate: rate, Rep: rep, Requests: m.Requests, Success: m.Success,
		P50us: us(m.Latencies.P50), P90us: us(m.Latencies.P90), P99us: us(m.Latencies.P99),
		P999us: us(m.Latencies.Quantile(0.999)), MaxUs: us(m.Latencies.Max),
	}
	if pid > 0 && m.Requests > 0 {
		after, err := cpuTicks(pid)
		if err != nil {
			return r, err
		}
		r.CPUPer1k = (after - before) / float64(m.Requests) * 1000
	}
	return r, nil
}

func us(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1000
}

func streamProbe(ctx context.Context, name, url string, body []byte, n int) (streamResult, error) {
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 4}}
	var ttfb, total []float64
	for i := range n + 20 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return streamResult{}, fmt.Errorf("stream request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return streamResult{}, fmt.Errorf("stream do: %w", err)
		}
		first := time.Duration(0)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if first == 0 && strings.Contains(sc.Text(), `"content":"`) &&
				!strings.Contains(sc.Text(), `"content":""`) {
				first = time.Since(start)
			}
		}
		elapsed := time.Since(start)
		resp.Body.Close()
		if i >= 20 {
			ttfb = append(ttfb, us(first))
			total = append(total, us(elapsed))
		}
	}
	return streamResult{
		Target:   name,
		N:        n,
		TTFBp50:  pct(ttfb, 0.5),
		TTFBp99:  pct(ttfb, 0.99),
		TotalP50: pct(total, 0.5),
		TotalP99: pct(total, 0.99),
	}, nil
}

func pct(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	return s[min(int(q*float64(len(s))), len(s)-1)]
}

func readFirst(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(b))
}

func cpuModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "model name"); ok {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), ":"))
		}
	}
	return "unknown"
}

func logicalCPUs() int {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return runtime.NumCPU()
	}
	return strings.Count(string(b), "\nprocessor") + 1
}

func git(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func environment(o options) map[string]string {
	mem := "unknown"
	if f, err := os.ReadFile("/proc/meminfo"); err == nil {
		mem = strings.Fields(strings.SplitN(string(f), "\n", 2)[0])[1] + " kB"
	}
	return map[string]string{
		"cpu":          cpuModel(),
		"logical_cpus": strconv.Itoa(logicalCPUs()),
		"memory":       mem,
		"kernel":       readFirst("/proc/sys/kernel/osrelease"),
		"governor":     readFirst("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor"),
		"boost":        readFirst("/sys/devices/system/cpu/cpufreq/boost"),
		"go":           runtime.Version(),
		"gomaxprocs":   "4 (maskwire, mockllm)",
		"pinning":      "maskwire " + o.proxyCPUs + ", mockllm " + o.mockCPUs + ", loadgen inherited",
		"git_sha":      git("rev-parse", "--short", "HEAD"),
		"git_dirty":    strconv.FormatBool(git("status", "--porcelain") != ""),
		"duration":     o.duration.String(),
		"warmup":       o.warmup.String(),
		"reps":         strconv.Itoa(o.reps),
		"date":         time.Now().UTC().Format(time.RFC3339),
		"network":      "same-host loopback, Go mock upstream with zero latency",
	}
}

func median(rs []runResult) runResult {
	s := slices.Clone(rs)
	slices.SortFunc(s, func(a, b runResult) int {
		switch {
		case a.P99us < b.P99us:
			return -1
		case a.P99us > b.P99us:
			return 1
		}
		return 0
	})
	return s[len(s)/2]
}

func report(s summary) string {
	var sb strings.Builder
	e := s.Env
	fmt.Fprintf(
		&sb,
		"# maskwire load test\n\n%s, %s logical CPUs, %s RAM, Linux %s, governor=%s, boost=%s, %s, GOMAXPROCS %s.\n",
		e["cpu"],
		e["logical_cpus"],
		e["memory"],
		e["kernel"],
		e["governor"],
		e["boost"],
		e["go"],
		e["gomaxprocs"],
	)
	fmt.Fprintf(
		&sb,
		"Pinning: %s. %s. Commit %s (dirty=%s), %s.\n\n",
		e["pinning"],
		e["network"],
		e["git_sha"],
		e["git_dirty"],
		e["date"],
	)
	fmt.Fprintf(
		&sb,
		"Non-streaming `POST /v1/chat/completions`, %d-byte request with 6 PII entities, vegeta open-model constant rate, "+
			"%s warm-up discarded, %s measured, median of %s runs (by p99).\n\n",
		s.Payload,
		e["warmup"],
		e["duration"],
		e["reps"],
	)
	sb.WriteString(
		"| rate (req/s) | direct p50 | direct p99 | maskwire p50 | maskwire p99 | added p50 | added p99 | maskwire p99.9 | success | CPU ms / 1k req |\n",
	)
	sb.WriteString("|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	rates := []int{}
	for _, r := range s.Runs {
		if !slices.Contains(rates, r.Rate) {
			rates = append(rates, r.Rate)
		}
	}
	for _, rate := range rates {
		var d, p []runResult
		for _, r := range s.Runs {
			if r.Rate != rate {
				continue
			}
			if r.Target == "direct" {
				d = append(d, r)
			} else {
				p = append(p, r)
			}
		}
		md, mp := median(d), median(p)
		fmt.Fprintf(
			&sb,
			"| %d | %.0f µs | %.0f µs | %.0f µs | %.0f µs | %.0f µs | %.0f µs | %.0f µs | %.2f%% | %.0f |\n",
			rate,
			md.P50us,
			md.P99us,
			mp.P50us,
			mp.P99us,
			mp.P50us-md.P50us,
			mp.P99us-md.P99us,
			mp.P999us,
			mp.Success*100,
			mp.CPUPer1k,
		)
	}
	sb.WriteString(
		"\nStreaming (`stream: true`, mockllm sends 1-7 byte content deltas with no delay), sequential requests:\n\n",
	)
	sb.WriteString(
		"| target | requests | TTFB p50 | TTFB p99 | full stream p50 | full stream p99 |\n|---|---:|---:|---:|---:|---:|\n",
	)
	for _, st := range s.Streams {
		fmt.Fprintf(
			&sb,
			"| %s | %d | %.0f µs | %.0f µs | %.0f µs | %.0f µs |\n",
			st.Target,
			st.N,
			st.TTFBp50,
			st.TTFBp99,
			st.TotalP50,
			st.TotalP99,
		)
	}
	return sb.String()
}

func write(dir string, s summary) error {
	host, err := os.Hostname()
	if err != nil {
		host = "host"
	}
	out := filepath.Join(dir, time.Now().UTC().Format("2006-01-02")+"-"+host+"-"+s.Env["git_sha"])
	if err := os.MkdirAll(out, 0o750); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	js, err := json.Marshal(s, json.Deterministic(true))
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	rep := report(s)
	if err := errors.Join(
		os.WriteFile(filepath.Join(out, "summary.json"), js, 0o600),
		os.WriteFile(filepath.Join(out, "report.md"), []byte(rep), 0o600),
	); err != nil {
		return fmt.Errorf("write results: %w", err)
	}
	_, err = io.WriteString(os.Stdout, rep+"\nwritten to "+out+"\n")
	if err != nil {
		return fmt.Errorf("print report: %w", err)
	}
	return nil
}
