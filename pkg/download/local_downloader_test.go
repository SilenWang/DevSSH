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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDownloader(t *testing.T) *LocalDownloader {
	t.Helper()
	cacheDir := t.TempDir()
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, 0)
	return NewLocalDownloader(cacheDir, logger)
}

func TestGetVSCodeDownloadURL_Default(t *testing.T) {
	url := GetVSCodeDownloadURL("1.116.02821", "linux", "amd64")
	assert.Equal(t, "https://github.com/VSCodium/vscodium/releases/download/1.116.02821/vscodium-reh-web-linux-x64-1.116.02821.tar.gz", url)
}

func TestGetVSCodeDownloadURL_DarwinARM64(t *testing.T) {
	url := GetVSCodeDownloadURL("1.116.02821", "darwin", "arm64")
	assert.Equal(t, "https://github.com/VSCodium/vscodium/releases/download/1.116.02821/vscodium-reh-web-darwin-arm64-1.116.02821.tar.gz", url)
}

func TestGetVSCodeDownloadURL_EnvOverride(t *testing.T) {
	t.Setenv(config.EnvVSCodeDownloadURL, "http://localhost:9999/vscode-{{version}}-{{os}}-{{arch}}.tar.gz")
	url := GetVSCodeDownloadURL("1.0.0", "linux", "amd64")
	assert.Equal(t, "http://localhost:9999/vscode-1.0.0-linux-amd64.tar.gz", url)
}

func TestGetVSCodeDownloadURL_EnvOverrideThenDefault(t *testing.T) {
	os.Setenv(config.EnvVSCodeDownloadURL, "http://localhost:9999/test")
	urlWithEnv := GetVSCodeDownloadURL("1.0", "linux", "amd64")
	assert.Equal(t, "http://localhost:9999/test", urlWithEnv)
	os.Unsetenv(config.EnvVSCodeDownloadURL)

	urlDefault := GetVSCodeDownloadURL("1.116.02821", "linux", "amd64")
	assert.Equal(t, "https://github.com/VSCodium/vscodium/releases/download/1.116.02821/vscodium-reh-web-linux-x64-1.116.02821.tar.gz", urlDefault)
}

func TestGetVSCodeDownloadURL_UnsupportedArch(t *testing.T) {
	url := GetVSCodeDownloadURL("1.0", "linux", "riscv64")
	assert.Contains(t, url, "riscv64")
}

func TestDownloadAndCache(t *testing.T) {
	d := newTestDownloader(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test content"))
	}))
	defer ts.Close()

	path, err := d.Download(ts.URL)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(data))
}

func TestDownloadCacheHit(t *testing.T) {
	d := newTestDownloader(t)
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("cached content"))
	}))
	defer ts.Close()

	path1, err := d.Download(ts.URL)
	require.NoError(t, err)

	path2, err := d.Download(ts.URL)
	require.NoError(t, err)

	assert.Equal(t, path1, path2, "should return same cache path")
	assert.Equal(t, 1, callCount, "should only call server once")
}

func TestDownloadEmptyURL(t *testing.T) {
	d := newTestDownloader(t)
	_, err := d.Download("")
	assert.Error(t, err)
}

func TestDownloadHTTPError(t *testing.T) {
	d := newTestDownloader(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := d.Download(ts.URL)
	assert.Error(t, err)
}

func TestCachePath(t *testing.T) {
	d := newTestDownloader(t)
	path, err := d.getCachePath("https://example.com/file.tar.gz")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, ".tar.gz"))
	assert.True(t, strings.HasPrefix(path, d.cacheDir))
}

func TestCachePathWithoutExtension(t *testing.T) {
	d := newTestDownloader(t)
	path, err := d.getCachePath("https://example.com/binary")
	require.NoError(t, err)
	assert.Equal(t, "", filepath.Ext(path))
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
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractExtFromURL(tt.url))
		})
	}
}

func TestCleanOldCache(t *testing.T) {
	d := newTestDownloader(t)
	oldFile := filepath.Join(d.cacheDir, "old-file")
	os.WriteFile(oldFile, []byte("old"), 0644)

	newFile := filepath.Join(d.cacheDir, "new-file")
	os.WriteFile(newFile, []byte("new"), 0644)

	err := d.CleanOldCache(0)
	assert.NoError(t, err)
}
