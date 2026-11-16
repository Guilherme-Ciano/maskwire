package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/config"
)

var (
	errUndocumented = errors.New("config keys without a description")
	errStale        = errors.New("descriptions for keys that no longer exist")
	errOutdated     = errors.New("docs/configuration.md is out of date, run make configdoc")
)

type row struct {
	key, typ, def string
}

func walk(t reflect.Type, v reflect.Value, prefix string, out []row) []row {
	for i := range t.NumField() {
		f := t.Field(i)
		name := f.Tag.Get("koanf")
		if name == "" {
			continue
		}
		key := prefix + name
		var fv reflect.Value
		if v.IsValid() {
			fv = v.Field(i)
		}
		if elem, suffix, ok := nested(f.Type); ok {
			if elem == f.Type {
				out = walk(elem, fv, key+suffix, out)
			} else {
				out = walk(elem, reflect.Value{}, key+suffix, out)
			}
			continue
		}
		out = append(out, row{key: key, typ: typeName(f.Type), def: format(fv)})
	}
	return out
}

func nested(t reflect.Type) (reflect.Type, string, bool) {
	k := t.Kind()
	switch {
	case k == reflect.Struct && t != reflect.TypeFor[time.Duration]():
		return t, ".", true
	case k == reflect.Slice && t.Elem().Kind() == reflect.Struct:
		return t.Elem(), "[].", true
	case k == reflect.Map && t.Elem().Kind() == reflect.Struct:
		return t.Elem(), ".<TYPE>.", true
	}
	return nil, "", false
}

func typeName(t reflect.Type) string {
	switch {
	case t == reflect.TypeFor[time.Duration]():
		return "duration"
	case t.Kind() == reflect.Slice:
		return "list of " + typeName(t.Elem())
	case t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64:
		return "number"
	case t.Kind() >= reflect.Int && t.Kind() <= reflect.Int64:
		return "integer"
	default:
		return t.Kind().String()
	}
}

func format(v reflect.Value) string {
	if !v.IsValid() || v.IsZero() {
		return ""
	}
	switch x := v.Interface().(type) {
	case time.Duration:
		return x.String()
	case []string:
		return strings.Join(x, ",")
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}

func render(rows []row) ([]byte, error) {
	var missing, stale []string
	desc := descriptions()
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.key] = true
		if desc[r.key] == "" {
			missing = append(missing, r.key)
		}
	}
	for k := range desc {
		if !seen[k] {
			stale = append(stale, k)
		}
	}
	slices.Sort(stale)
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", errUndocumented, strings.Join(missing, ", "))
	}
	if len(stale) > 0 {
		return nil, fmt.Errorf("%w: %s", errStale, strings.Join(stale, ", "))
	}
	var b bytes.Buffer
	b.WriteString(header)
	b.WriteString("| Key | Type | Default | Description |\n|---|---|---|---|\n")
	for _, r := range rows {
		def := ""
		if r.def != "" {
			def = "`" + r.def + "`"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", r.key, r.typ, def, desc[r.key])
	}
	return b.Bytes(), nil
}

func run(args []string) error {
	fs := flag.NewFlagSet("configdoc", flag.ContinueOnError)
	out := fs.String("out", "docs/configuration.md", "output file")
	check := fs.Bool("check", false, "fail if the output file is not up to date")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	d := config.Defaults()
	doc, err := render(walk(reflect.TypeOf(d), reflect.ValueOf(d), "", nil))
	if err != nil {
		return err
	}
	if *check {
		cur, err := os.ReadFile(*out)
		if err != nil {
			return fmt.Errorf("read %s: %w", *out, err)
		}
		if !bytes.Equal(cur, doc) {
			return errOutdated
		}
		return nil
	}
	if err := os.WriteFile(*out, doc, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
