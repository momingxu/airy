package scanner

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResourceRootsForAlbumDepth(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"A/one", "A/two", "B/one", ".hidden/one", "Empty"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		depth uint
		want  []string
	}{
		{1, []string{root}},
		{2, []string{filepath.Join(root, "A"), filepath.Join(root, "B")}},
		{3, []string{}},
	} {
		got, err := ResourceRootsForAlbumDepth(context.Background(), root, tc.depth)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("depth %d: got %v want %v", tc.depth, got, tc.want)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "A/one/deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResourceRootsForAlbumDepth(context.Background(), root, 3)
	if err != nil || !reflect.DeepEqual(got, []string{filepath.Join(root, "A/one")}) {
		t.Fatalf("deep roots %v: %v", got, err)
	}
	for _, depth := range []uint{0, 11} {
		if _, err := ResourceRootsForAlbumDepth(context.Background(), root, depth); err == nil {
			t.Fatal("invalid depth accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ResourceRootsForAlbumDepth(ctx, root, 2); err == nil {
		t.Fatal("cancellation ignored")
	}
}
