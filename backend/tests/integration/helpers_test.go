//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"devsec-contest01/internal/config"
	"devsec-contest01/internal/server"
)

const adminToken = "integration-admin-token"

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type fixture struct {
	server *httptest.Server
	t      *testing.T
	pool   *pgxpool.Pool
	url    string
	client *http.Client
	cfg    config.Config
}

// Each test owns a new database. Cleanup is registered before migrations/server startup.
func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required; use ./tests/integration/run.sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	name := "contest_test_" + hex.EncodeToString(random[:])
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrations, err := filepath.Glob(filepath.Join(backendRoot(), "db/migrations/*.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find migrations: %v (%d files)", err, len(migrations))
	}
	for _, path := range migrations {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	f := &fixture{
		t:      t,
		pool:   pool,
		client: &http.Client{Timeout: 10 * time.Second},
		cfg: config.Config{
			AdminToken:             adminToken,
			SessionTTLHours:        336,
			ContestDurationSeconds: 5400,
		},
	}
	f.startServer()
	return f
}

func backendRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func (f *fixture) startServer() {
	f.t.Helper()
	if f.server != nil {
		f.server.Close()
	}
	router, err := server.New(context.Background(), f.cfg, f.pool)
	if err != nil {
		f.t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	f.t.Cleanup(srv.Close)
	f.url = srv.URL
	f.server = srv
}

type response struct {
	body   []byte
	header http.Header
}

func (f *fixture) request(method, path, token, contentType string, body io.Reader, want int) response {
	f.t.Helper()
	req, err := http.NewRequest(method, f.url+path, body)
	if err != nil {
		f.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := f.client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	if res.StatusCode != want {
		f.t.Fatalf("%s %s: status %d, want %d: %.500s", method, path, res.StatusCode, want, data)
	}
	return response{
		body:   data,
		header: res.Header,
	}
}

func decode[T any](t *testing.T, r response) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(r.body, &result); err != nil {
		t.Fatalf("decode response: %v: %.500s", err, r.body)
	}
	return result
}

func (f *fixture) json(method, path, token, body string, want int) response {
	f.t.Helper()
	return f.request(method, path, token, "application/json", strings.NewReader(body), want)
}

func (f *fixture) register(name string) string {
	f.t.Helper()
	data, err := json.Marshal(map[string]string{"name": name, "group": "CS-101"})
	if err != nil {
		f.t.Fatal(err)
	}
	result := decode[struct{ Token string }](f.t, f.json("POST", "/api/v1/register", "", string(data), 201))
	if len(result.Token) != 64 {
		f.t.Fatalf("unexpected token length: %d", len(result.Token))
	}
	return result.Token
}

func (f *fixture) upload(task, token string, data []byte, want int) response {
	f.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "output.txt")
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		f.t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		f.t.Fatal(err)
	}
	return f.request("POST", "/api/v1/tasks/"+task+"/submit", token, writer.FormDataContentType(), &body, want)
}

func readTaskFile(t *testing.T, task, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(backendRoot(), "internal/tasks", task, "data", file))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type me struct {
	Participant struct {
		ID    int64
		Name  string
		Group string
	}
	SelectedTaskID *string
	Scores         map[string]float64
}

type verdict struct {
	TaskID string
	Passed bool
	Earned float64
}

func (f *fixture) me(token string) me {
	f.t.Helper()
	return decode[me](f.t, f.json("GET", "/api/v1/me", token, "", 200))
}

func (f *fixture) selectTask(task, token string, want int) {
	f.t.Helper()
	r := f.json("POST", "/api/v1/tasks/"+task+"/select", token, "", want)
	if want == 200 {
		selected := decode[struct{ SelectedTaskID string }](f.t, r)
		if selected.SelectedTaskID != task {
			f.t.Fatalf("selected %q, want %q", selected.SelectedTaskID, task)
		}
	}
}
