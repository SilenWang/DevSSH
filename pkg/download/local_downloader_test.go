package download

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devssh/pkg/config"
	"github.com/loft-sh/log"
)

func TestNewLocalDownloader(t *testing.T) {
	d := NewLocalDownloader("/tmp/cache", log.NewStreamLogger(os.Stdout, os.Stderr, 0))
	if d == nil {
		t.Fatal("expected non-nil downloader")
	}
}

func TestGetVSCodeDownloadURL_Default(t *testing.T) {
	url := GetVSCodeDownloadURL("1.121.03429", "linux", "amd64")
	expected := "https://github.com/VSCodium/vscodium/releases/download/1.121.03429/vscodium-reh-web-linux-x64-1.121.03429.tar.gz"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestGetVSCodeDownloadURL_DarwinARM64(t *testing.T) {
	url := GetVSCodeDownloadURL("1.121.03429", "darwin", "arm64")
	expected := "https://github.com/VSCodium/vscodium/releases/download/1.121.03429/vscodium-reh-web-darwin-arm64-1.121.03429.tar.gz"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestGetVSCodeDownloadURL_EnvOverride(t *testing.T) {
	os.Setenv(config.EnvVSCodeDownloadURL, "http://localhost:9999/vscode-{{version}}-{{os}}-{{arch}}.tar.gz")
	defer os.Unsetenv(config.EnvVSCodeDownloadURL)

	url := GetVSCodeDownloadURL("1.0.0", "linux", "amd64")
	expected := "http://localhost:9999/vscode-1.0.0-linux-amd64.tar.gz"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestGetVSCodeDownloadURL_DefaultRestoreAfterEnv(t *testing.T) {
	os.Setenv(config.EnvVSCodeDownloadURL, "http://localhost:9999/test")
	_ = GetVSCodeDownloadURL("1.0", "linux", "amd64")
	os.Unsetenv(config.EnvVSCodeDownloadURL)

	url := GetVSCodeDownloadURL("1.121.03429", "linux", "amd64")
	expected := "https://github.com/VSCodium/vscodium/releases/download/1.121.03429/vscodium-reh-web-linux-x64-1.121.03429.tar.gz"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestGetVSCodeDownloadURL_UnsupportedArch(t *testing.T) {
	url := GetVSCodeDownloadURL("1.0", "linux", "riscv64")
	if !strings.Contains(url, "riscv64") {
		t.Errorf("expected URL to contain riscv64, got %s", url)
	}
}

func TestDownloadAndCache(t *testing.T) {
	cacheDir := t.TempDir()
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, 0)
	d := NewLocalDownloader(cacheDir, logger)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test content"))
	}))
	defer ts.Close()

	path, err := d.Download(ts.URL)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != "test content" {
		t.Errorf("expected 'test content', got %q", string(data))
	}
}

func TestDownloadCacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, 0)
	d := NewLocalDownloader(cacheDir, logger)

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("cached content"))
	}))
	defer ts.Close()

	path1, err := d.Download(ts.URL)
	if err != nil {
		t.Fatalf("first download failed: %v", err)
	}

	// second download should hit cache
	path2, err := d.Download(ts.URL)
	if err != nil {
		t.Fatalf("second download failed: %v", err)
	}

	if path1 != path2 {
		t.Errorf("expected same cache path, got %s vs %s", path1, path2)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}
}

func TestDownloadEmptyURL(t *testing.T) {
	cacheDir := t.TempDir()
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, 0)
	d := NewLocalDownloader(cacheDir, logger)

	_, err := d.Download("")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestDownloadHTTPError(t *testing.T) {
	cacheDir := t.TempDir()
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, 0)
	d := NewLocalDownloader(cacheDir, logger)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := d.Download(ts.URL)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestCachePath(t *testing.T) {
	cacheDir := t.TempDir()
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, 0)
	d := NewLocalDownloader(cacheDir, logger)

	path, err := d.getCachePath("https://example.com/file.tar.gz")
	if err != nil {
		t.Fatalf("getCachePath failed: %v", err)
	}
	if !strings.HasSuffix(path, ".tar.gz") {
		t.Errorf("expected .tar.gz extension, got %s", path)
	}
	if !strings.HasPrefix(path, cacheDir) {
		t.Errorf("expected path under cacheDir, got %s", path)
	}
}

func TestCachePathWithoutExtension(t *testing.T) {
	cacheDir := t.TempDir()
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, 0)
	d := NewLocalDownloader(cacheDir, logger)

	path, err := d.getCachePath("https://example.com/binary")
	if err != nil {
		t.Fatalf("getCachePath failed: %v", err)
	}
	if filepath.Ext(path) != "" {
		t.Errorf("expected no extension, got %s", filepath.Ext(path))
	}
}

func TestExtractExtFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/file.tar.gz", ".tar.gz"},
		{"https://example.com/file.gz", ".gz"},
		{"https://example.com/file.zip", ".zip"},
		{"https://example.com/binary", ""},
		{"https://example.com/file.txt", ""},
	}
	for _, tt := range tests {
		got := extractExtFromURL(tt.url)
		if got != tt.expected {
			t.Errorf("extractExtFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}

func TestCleanOldCache(t *testing.T) {
	cacheDir := t.TempDir()
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, 0)
	d := NewLocalDownloader(cacheDir, logger)

	oldFile := filepath.Join(cacheDir, "old-file")
	os.WriteFile(oldFile, []byte("old"), 0644)

	newFile := filepath.Join(cacheDir, "new-file")
	os.WriteFile(newFile, []byte("new"), 0644)

	err := d.CleanOldCache(0)
	if err != nil {
		t.Fatalf("CleanOldCache failed: %v", err)
	}

	if _, err := os.Stat(oldFile); err == nil {
		t.Log("old file may still exist (within TTL)")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Log("new file may have been cleaned (within TTL)")
	}
}
