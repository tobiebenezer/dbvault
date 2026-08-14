package chunking

import "testing"

func TestPagesPerChunk(t *testing.T) {
	if PagesPerChunk(4096, 1024*1024) != 256 {
		t.Fatalf("wrong pages per chunk")
	}
	if PagesPerChunk(4096, 1) != 1 {
		t.Fatalf("minimum should be one")
	}
}
