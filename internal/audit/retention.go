package audit

import (
	"bufio"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/jsonx"
)

const (
	ledgerName  = "_retention.ledger"
	pruneEvery  = time.Hour
	retentionID = "retention"
)

type ledgerRecord struct {
	Type           string  `json:"type"`
	DeletedThrough string  `json:"deleted_through"`
	LinkHead       *string `json:"link_head"`
}

func retentionBody(dst []byte, ts time.Time, through string, link *string) []byte {
	dst = headerBody(dst, retentionID, ts)
	dst = appendKV(dst, "deleted_through")
	dst = jsonx.Quote(dst, []byte(through))
	dst = appendKV(dst, "link_head")
	if link == nil {
		return append(dst, "null"...)
	}
	return jsonx.Quote(dst, []byte(*link))
}

func ledgerPath(dir, tenant string) string {
	return filepath.Join(dir, tenant, ledgerName)
}

func (w *Writer) prune() {
	now := w.opts.Now()
	if w.opts.RetentionDays <= 0 || now.Sub(w.lastPrune) < pruneEvery {
		return
	}
	w.lastPrune = now
	cutoff := now.UTC().AddDate(0, 0, -w.opts.RetentionDays).Format(dayLayout)
	tenants, err := os.ReadDir(w.opts.Dir)
	if err != nil {
		w.log.Error("audit retention: list tenants", slog.String("error", err.Error()))
		return
	}
	for _, t := range tenants {
		if !t.IsDir() || !ValidTenant(t.Name()) {
			continue
		}
		if err := w.pruneTenant(t.Name(), cutoff, now); err != nil {
			w.log.Error("audit retention failed", slog.String("tenant", t.Name()), slog.String("error", err.Error()))
		}
	}
}

func (w *Writer) pruneTenant(tenant, cutoff string, now time.Time) error {
	files, err := dayFiles(w.opts.Dir, tenant)
	if err != nil {
		return err
	}
	var victims []string
	next := ""
	for _, path := range files {
		day := dayOf(path)
		if day >= cutoff || w.chains[tenant+"/"+day] != nil {
			next = path
			break
		}
		victims = append(victims, path)
	}
	if len(victims) == 0 {
		return nil
	}
	last := victims[len(victims)-1]
	link, ok, err := w.linkHead(last, next)
	if err != nil {
		return err
	}
	var linkPtr *string
	if ok {
		linkPtr = &link
	}
	if err := w.appendLedger(tenant, now, dayOf(last), linkPtr); err != nil {
		return err
	}
	var errs []error
	for _, v := range victims {
		errs = append(errs, os.Remove(v))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("remove expired audit files: %w", err)
	}
	return nil
}

func (w *Writer) linkHead(last, next string) (head string, found bool, err error) {
	prev, err := VerifyFile(last, w.sealer)
	if err != nil {
		return "", false, err
	}
	if next == "" {
		return "", false, nil
	}
	cur, err := VerifyFile(next, w.sealer)
	if err != nil {
		return "", false, err
	}
	if err := checkLink(&cur, &prev, nil); err != nil {
		return "", false, err
	}
	if cur.Open == nil || cur.Open.PrevDayHead == nil {
		return "", false, nil
	}
	return *cur.Open.PrevDayHead, true, nil
}

func (w *Writer) appendLedger(tenant string, now time.Time, through string, link *string) error {
	path := ledgerPath(w.opts.Dir, tenant)
	f, err := os.OpenFile(filepath.Clean(path), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open retention ledger: %w", err)
	}
	prev := w.sealer.Genesis(tenant, retentionID)
	head, torn, err := tail(f)
	if err != nil || torn > 0 {
		return errors.Join(fmt.Errorf("retention ledger tail: %w", errors.Join(err, ErrBrokenChain)), f.Close())
	}
	if head != nil {
		prev = *head
	}
	line, _ := w.sealer.seal(retentionBody(nil, now, through, link), &prev)
	_, werr := f.Write(line)
	if err := errors.Join(werr, f.Sync(), f.Close()); err != nil {
		return fmt.Errorf("write retention ledger: %w", err)
	}
	return nil
}

type ledger map[string]*string

func readLedger(dir, tenant string, sealer *Sealer) (ledger, error) {
	path := ledgerPath(dir, tenant)
	f, err := os.Open(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return ledger{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open retention ledger: %w", err)
	}
	defer f.Close()
	if _, err := VerifyReader(path, f, sealer, sealer.Genesis(tenant, retentionID)); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("rewind retention ledger: %w", err)
	}
	out := ledger{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r ledgerRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil || r.Type != retentionID {
			return nil, &ChainError{path, 0, "malformed retention record"}
		}
		out[r.DeletedThrough] = r.LinkHead
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read retention ledger: %w", err)
	}
	return out, nil
}

func (l ledger) covers(prevDay string, head *string) bool {
	link, ok := l[prevDay]
	return ok && link != nil && head != nil && *link == *head
}

func (l ledger) deletedThrough() string {
	last := ""
	for d := range l {
		last = max(last, d)
	}
	return last
}
