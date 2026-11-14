package scanner

import (
	"context"
	"fmt"
	"path/filepath"
)

// ResourceRootsForAlbumDepth groups the selected album directories by their
// immediate parent. Depth one also allows registering an initially empty root.
func ResourceRootsForAlbumDepth(ctx context.Context, root string, depth uint) ([]string, error) {
	if depth < 1 || depth > 10 {
		return nil, fmt.Errorf("相册目录深度必须在 1 到 10 之间")
	}
	if depth == 1 {
		return []string{root}, nil
	}
	albums, err := discoverAlbumPaths(ctx, root, int(depth))
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0)
	seen := make(map[string]bool)
	for _, album := range albums {
		parent := filepath.Dir(album)
		if !seen[parent] {
			seen[parent] = true
			roots = append(roots, parent)
		}
	}
	return roots, nil
}
