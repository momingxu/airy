package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// resolveBrowsePath checks both the logical path and symlink target.
func resolveBrowsePath(root, path string) (string, error) {
	if path == "" {
		path = root
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", os.ErrPermission
	}
	return resolved, nil
}

func browseDirectoriesHandler(root string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.DefaultQuery("path", root)
		// Reject lexical traversal even when an external symlink points back inside.
		rel, err := filepath.Rel(root, filepath.Clean(path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			c.JSON(http.StatusForbidden, gin.H{"error": "只能浏览 /mnt 下的目录"})
			return
		}
		resolved, err := resolveBrowsePath(root, path)
		if err != nil {
			status := http.StatusBadRequest
			if os.IsPermission(err) {
				status = http.StatusForbidden
			}
			c.JSON(status, gin.H{"error": "目录不可访问或不在 /mnt 内"})
			return
		}
		entries, err := os.ReadDir(resolved)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取目录失败"})
			return
		}
		type item struct {
			Name       string    `json:"name"`
			Path       string    `json:"path"`
			ModifiedAt time.Time `json:"modified_at"`
		}
		items := make([]item, 0)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			child := filepath.Join(path, entry.Name())
			target, err := resolveBrowsePath(root, child)
			if err != nil {
				continue
			}
			info, err := os.Stat(target)
			if err != nil || !info.IsDir() {
				continue
			}
			items = append(items, item{entry.Name(), child, info.ModTime()})
		}
		c.JSON(http.StatusOK, gin.H{"path": filepath.Clean(path), "directories": items})
	}
}
