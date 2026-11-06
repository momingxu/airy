package main

import (
	"io/fs"
	"testing"
)

func TestEmbeddedFrontendContainsIndex(t *testing.T) {
	index, err := fs.ReadFile(embeddedFrontendFS(), "index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	if len(index) == 0 {
		t.Fatal("embedded index is empty")
	}
}
