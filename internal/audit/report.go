package audit

import (
	"bufio"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type requestRecord struct {
	Type       string            `json:"type"`
	TS         string            `json:"ts"`
	Tenant     string            `json:"tenant"`
	Route      string            `json:"route"`
	Upstream   string            `json:"upstream"`
	Model      string            `json:"model"`
	Mode       string            `json:"mode"`
	Status     int               `json:"status"`
	Streaming  bool              `json:"streaming"`
	Detections map[string]uint64 `json:"detections"`
	Generated  map[string]uint64 `json:"response_detections"`
	Actions    map[string]uint64 `json:"actions"`
	Restore    map[string]uint64 `json:"restore"`
}

type TenantSummary struct {
	Tenant             string            `json:"tenant"`
	From               string            `json:"from"`
	To                 string            `json:"to"`
	Requests           uint64            `json:"requests"`
	Streaming          uint64            `json:"streaming"`
	Modes              map[string]uint64 `json:"modes"`
	Statuses           map[string]uint64 `json:"statuses"`
	Routes             map[string]uint64 `json:"routes"`
	Recipients         map[string]uint64 `json:"recipients"`
	Models             map[string]uint64 `json:"models"`
	Categories         map[string]uint64 `json:"categories"`
	Actions            map[string]uint64 `json:"actions"`
	Restore            map[string]uint64 `json:"restore"`
	ResponseCategories map[string]uint64 `json:"response_categories"`
}

type Summary struct {
	From    string          `json:"from"`
	To      string          `json:"to"`
	Files   int             `json:"files_verified"`
	Records int             `json:"records_verified"`
	Tenants []TenantSummary `json:"tenants"`
}

func newTenantSummary(id string) *TenantSummary {
	return &TenantSummary{
		Tenant: id, Modes: map[string]uint64{}, Statuses: map[string]uint64{}, Routes: map[string]uint64{},
		Recipients: map[string]uint64{}, Models: map[string]uint64{}, Categories: map[string]uint64{},
		Actions: map[string]uint64{}, Restore: map[string]uint64{}, ResponseCategories: map[string]uint64{},
	}
}

func addAll(dst, src map[string]uint64) {
	for k, v := range src {
		if v > 0 {
			dst[k] += v
		}
	}
}

func (t *TenantSummary) add(r *requestRecord) {
	t.Requests++
	if r.Streaming {
		t.Streaming++
	}
	t.Modes[r.Mode]++
	t.Statuses[strconv.Itoa(r.Status)]++
	t.Routes[r.Route]++
	t.Recipients[r.Upstream]++
	if r.Model != "" {
		t.Models[r.Model]++
	}
	addAll(t.Categories, r.Detections)
	addAll(t.Actions, r.Actions)
	addAll(t.Restore, r.Restore)
	addAll(t.ResponseCategories, r.Generated)
	day := r.TS[:min(len(r.TS), len(dayLayout))]
	if t.From == "" || day < t.From {
		t.From = day
	}
	if day > t.To {
		t.To = day
	}
}

func Summarize(dir string, sealer *Sealer, anchors Anchors, from, to string) (Summary, error) {
	rep, err := VerifyDir(dir, sealer, anchors)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{From: from, To: to, Files: rep.Files, Records: rep.Records}
	tenants, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return sum, fmt.Errorf("list tenants: %w", err)
	}
	for _, t := range tenants {
		files, err := dayFiles(dir, filepath.Base(t))
		if err != nil {
			return sum, err
		}
		ts := newTenantSummary(filepath.Base(t))
		for _, path := range files {
			day := dayOf(path)
			if (from != "" && day < from) || (to != "" && day > to) {
				continue
			}
			if err := summarizeFile(path, ts); err != nil {
				return sum, err
			}
		}
		if ts.Requests > 0 {
			sum.Tenants = append(sum.Tenants, *ts)
		}
	}
	return sum, nil
}

func summarizeFile(path string, ts *TenantSummary) error {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		var r requestRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if r.Type == "request" {
			ts.add(&r)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func sortedKeys(m map[string]uint64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if m[a] != m[b] {
			if m[a] > m[b] {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	return keys
}

func writeTable(b *strings.Builder, title, col string, m map[string]uint64) {
	fmt.Fprintf(b, "\n**%s**\n\n| %s | Count |\n|---|---:|\n", title, col)
	if len(m) == 0 {
		b.WriteString("| none | 0 |\n")
		return
	}
	for _, k := range sortedKeys(m) {
		fmt.Fprintf(b, "| %s | %d |\n", k, m[k])
	}
}

func (s Summary) Markdown() string {
	var b strings.Builder
	period := "all retained days"
	if s.From != "" || s.To != "" {
		period = s.From + " to " + s.To
	}
	fmt.Fprintf(&b, "# maskwire processing summary\n\nPeriod: %s. Integrity: %d audit files and %d records verified "+
		"(HMAC chain, links between days, and anchors when given). The audit log holds no personal data values; "+
		"this summary counts categories, recipients and actions to support a record of processing activities "+
		"(GDPR Art. 30) and log retention duties. It is evidence for those records, not the records themselves.\n",
		period, s.Files, s.Records)
	for i := range s.Tenants {
		t := &s.Tenants[i]
		fmt.Fprintf(&b, "\n## Tenant `%s`\n\n%d requests (%d streamed) between %s and %s.\n",
			t.Tenant, t.Requests, t.Streaming, t.From, t.To)
		writeTable(&b, "Recipients (upstream processors)", "Upstream", t.Recipients)
		writeTable(&b, "Models", "Model", t.Models)
		writeTable(&b, "Categories of personal data detected in requests", "Entity type", t.Categories)
		writeTable(&b, "Safeguards applied", "Action", t.Actions)
		writeTable(&b, "Restore and guards", "Event", t.Restore)
		writeTable(&b, "Personal data generated by the model (response scan)", "Entity type", t.ResponseCategories)
		writeTable(&b, "Policy mode", "Mode", t.Modes)
		writeTable(&b, "Routes", "Route", t.Routes)
		writeTable(&b, "Response status", "Status", t.Statuses)
	}
	return b.String()
}
