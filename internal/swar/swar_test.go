package swar

import "testing"

func TestLowerOrSpaceExhaustive(t *testing.T) {
	t.Parallel()
	for c := range 256 {
		for pos := range Width {
			x := uint64(c) << (8 * pos)
			for k := range Width {
				if k != pos {
					x |= uint64('a') << (8 * k)
				}
			}
			plain, spaces := LowerOrSpace(x)
			b := byte(c)
			if got := plain>>(8*pos+7)&1 == 1; got != (b >= 'a' && b <= 'z' || b == ' ') {
				t.Fatalf("byte %#x at %d: plain %v", c, pos, got)
			}
			if got := spaces>>(8*pos+7)&1 == 1; got != (b == ' ') {
				t.Fatalf("byte %#x at %d: space %v", c, pos, got)
			}
		}
	}
}

func TestPlainPrefix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"abcdefgh", 8},
		{"abc defg", 8},
		{"Abcdefgh", 0},
		{"abcD efg", 3},
		{"abcdefg\xff", 7},
		{"\xffbcdefgh", 0},
	} {
		if got := PlainPrefix(Load([]byte(tc.in), 0)); got != tc.want {
			t.Errorf("%q: got %d want %d", tc.in, got, tc.want)
		}
	}
}

func TestEqAndRangeExhaustive(t *testing.T) {
	t.Parallel()
	neighbours := []byte{0x00, 'a', 0x7f, 0x80, 0xff, 'Z', '['}
	for c := range 256 {
		for _, n := range neighbours {
			for pos := range Width {
				var x uint64
				for k := range Width {
					v := uint64(n)
					if k == pos {
						v = uint64(c)
					}
					x |= v << (8 * k)
				}
				bit := func(m uint64) bool { return m>>(8*pos+7)&1 == 1 }
				b := byte(c)
				if bit(Eq(x, 0xE3)) != (b == 0xE3) || bit(Eq(x, '<')) != (b == '<') {
					t.Fatalf("Eq wrong for %#x at %d next to %#x", c, pos, n)
				}
				if bit(InRange(x, 'A', '\\')) != (b >= 'A' && b <= '\\') {
					t.Fatalf("InRange wrong for %#x at %d next to %#x", c, pos, n)
				}
			}
		}
	}
}
