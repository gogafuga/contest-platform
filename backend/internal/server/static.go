package server

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func mountFrontend(router *gin.Engine, dir string) error {
	var files fs.FS
	var index []byte
	if dir != "" {
		files = os.DirFS(dir)
		var err error
		index, err = fs.ReadFile(files, "index.html")
		if err != nil {
			return fmt.Errorf("load frontend: %w", err)
		}
	}
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if files == nil || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || path == "/api" || strings.HasPrefix(path, "/api/") || path == "/health" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		switch path {
		case "/", "/register", "/tasks", "/leaderboard", "/clock":
			c.Header("Cache-Control", "no-cache")
			c.Status(http.StatusOK)
			http.ServeContent(c.Writer, c.Request, "index.html", time.Time{}, bytes.NewReader(index))
			return
		}
		name := strings.TrimPrefix(path, "/")
		info, err := fs.Stat(files, name)
		if !fs.ValidPath(name) || err != nil || info.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}
		if strings.HasPrefix(path, "/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Status(http.StatusOK)
		http.FileServer(http.FS(files)).ServeHTTP(c.Writer, c.Request)
	})
	return nil
}
