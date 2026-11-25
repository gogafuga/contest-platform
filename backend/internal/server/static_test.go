package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFrontendRoutes(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"index.html": "<!doctype html><title>Contest</title>", "assets/app-123.js": "console.log('contest')"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	if err := mountFrontend(router, dir); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/register", "/tasks", "/leaderboard", "/clock"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if response.Code != 200 || !strings.Contains(response.Body.String(), "<title>Contest</title>") || response.Header().Get("Cache-Control") != "no-cache" {
				t.Fatalf("unexpected UI response: %d %s", response.Code, response.Body.String())
			}
		})
	}
	for _, path := range []string{"/api", "/api/v1/unknown", "/health", "/assets/missing.js", "/assets/", "/unknown", "/../index.html"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 404 || strings.Contains(response.Body.String(), "<title>") {
			t.Fatalf("%s: unexpected fallback: %d %s", path, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/assets/app-123.js", nil))
	if response.Code != 200 || response.Body.String() != "console.log('contest')" || !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset response: %d %s", response.Code, response.Body.String())
	}
	for _, method := range []string{"HEAD", "POST"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/tasks", nil))
		if method == "HEAD" && (response.Code != 200 || response.Body.Len() != 0) {
			t.Fatal("HEAD must return headers without content")
		}
		if method == "POST" && response.Code != 404 {
			t.Fatal("POST must not serve the SPA")
		}
	}
}

func TestFrontendConfiguration(t *testing.T) {
	if err := mountFrontend(gin.New(), t.TempDir()); err == nil {
		t.Fatal("configured frontend missing index.html must fail startup")
	}
	router := gin.New()
	if err := mountFrontend(router, ""); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/tasks", nil))
	if response.Code != 404 {
		t.Fatal("API-only mode must not serve frontend")
	}
}
