package main

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

func frontendHandler() gin.HandlerFunc {
	return frontendFileSystemHandler(embeddedFrontendFS())
}

func frontendFileSystemHandler(frontendFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		requestPath := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if requestPath != "" && fs.ValidPath(requestPath) {
			if info, err := fs.Stat(frontendFS, requestPath); err == nil && info.Mode().IsRegular() {
				switch {
				case strings.HasPrefix(c.Request.URL.Path, "/assets/"):
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				case c.Request.URL.Path == "/sw.js",
					c.Request.URL.Path == "/registerSW.js":
					c.Header("Cache-Control", "no-cache")
				case c.Request.URL.Path == "/manifest.webmanifest":
					c.Header("Cache-Control", "no-cache")
					c.Header("Content-Type", "application/manifest+json")
				}
				http.ServeFileFS(c.Writer, c.Request, frontendFS, requestPath)
				return
			}
		}

		if info, err := fs.Stat(frontendFS, "index.html"); err != nil || !info.Mode().IsRegular() {
			c.JSON(http.StatusNotFound, gin.H{"error": "frontend is not built"})
			return
		}
		c.Header("Cache-Control", "no-cache")
		http.ServeFileFS(c.Writer, c.Request, frontendFS, "index.html")
	}
}
