package detect

const maxHint = 64

func fieldSep(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c == ':' || c == '/' || c == ' '
}

func normalizeField(dst *[maxHint]byte, field []byte) int {
	n := 0
	var prev byte
	for _, orig := range field {
		if n >= maxHint-2 {
			break
		}
		c := orig
		switch {
		case fieldSep(c):
			c = ' '
		case c >= 'A' && c <= 'Z':
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
				dst[n] = ' '
				n++
			}
			c += 'a' - 'A'
		}
		if c != ' ' || (n > 0 && dst[n-1] != ' ') {
			dst[n] = c
			n++
		}
		prev = orig
	}
	if n > 0 && dst[n-1] != ' ' {
		dst[n] = ' '
		n++
	}
	return n
}

func negativeWord(w string) bool {
	switch w {
	case "order", "invoice", "sku", "trace", "span", "request", "uuid", "guid", "version", "build", "count",
		"amount", "price", "total", "quantity", "qty", "timestamp", "ts", "hash", "sha", "checksum", "ticket",
		"tracking", "page", "offset", "limit", "seq", "sequence", "product", "item", "batch", "lot":
		return true
	}
	return false
}

func negativeField(hint *[maxHint]byte, n int) bool {
	start := 0
	for i := 0; i <= n; i++ {
		if i < n && hint[i] != ' ' {
			continue
		}
		if i > start && negativeWord(string(hint[start:i])) {
			return true
		}
		start = i + 1
	}
	return false
}
