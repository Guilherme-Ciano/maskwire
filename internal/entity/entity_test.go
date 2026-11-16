package entity

import "testing"

func TestLabelRoundTrip(t *testing.T) {
	t.Parallel()
	for _, typ := range All() {
		got, ok := LabelFold([]byte(typ.Label()))
		want := typ
		if typ == IPPrivate {
			want = IP
		}
		if !ok || got != want {
			t.Errorf("%s: LabelFold(%q) = %s, %v", typ, typ.Label(), got, ok)
		}
		if lower, ok := LabelFold([]byte(typ.LabelLower())); !ok || lower != want {
			t.Errorf("%s: lower-case label not folded", typ)
		}
		if parsed, ok := Parse(typ.String()); !ok || parsed != typ {
			t.Errorf("%s: Parse round trip failed", typ)
		}
	}
	for _, bad := range []string{"", "FOO", "REDACTED", "EMAILS", "TOOLONGLABEL"} {
		if _, ok := LabelFold([]byte(bad)); ok {
			t.Errorf("%q must not fold", bad)
		}
	}
}
