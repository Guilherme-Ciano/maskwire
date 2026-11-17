package httpapi

import "testing"

func TestCopyBuffersRecycleOnlyTheirSize(t *testing.T) {
	t.Parallel()
	var c copyBuffers
	b := c.Get()
	if len(b) != copyBufferSize {
		t.Fatalf("len %d", len(b))
	}
	c.Put(b[:10])
	if got := c.Get(); len(got) != copyBufferSize {
		t.Fatalf("recycled buffer must come back full length, got %d", len(got))
	}
	c.Put(make([]byte, 7))
	if got := c.Get(); len(got) != copyBufferSize {
		t.Fatalf("foreign buffer handed out: %d", len(got))
	}
}
