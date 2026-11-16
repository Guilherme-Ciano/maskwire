package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const profile = `mode: atomic
a.go:1.1,2.2 3 1
a.go:3.1,4.2 1 0
a.go:3.1,4.2 1 2
b.go:1.1,2.2 6 0
`

func TestGate(t *testing.T) {
	t.Parallel()
	covered, total, err := parse(strings.NewReader(profile))
	if err != nil || covered != 4 || total != 10 {
		t.Fatalf("covered=%d total=%d err=%v", covered, total, err)
	}
	var out bytes.Buffer
	if err := gate(strings.NewReader(profile), &out, 40); err != nil || !strings.Contains(out.String(), "40.0%") {
		t.Fatalf("%q %v", out.String(), err)
	}
	if err := gate(strings.NewReader(profile), &out, 41); !errors.Is(err, errBelow) {
		t.Fatalf("got %v", err)
	}
	for _, bad := range []string{"mode: set\na.go:1.1 x\n", "mode: set\n"} {
		if err := gate(strings.NewReader(bad), &out, 0); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
}
