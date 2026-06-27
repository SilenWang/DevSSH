package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDETypeConstants(t *testing.T) {
	assert.Equal(t, IDEType("vscode"), IDETypeVSCode)
	assert.Equal(t, IDEType("code-server"), IDETypeCodeServer)
}

func TestIDEConfigDefaults(t *testing.T) {
	cfg := IDEConfig{Type: IDETypeVSCode, Port: 10081}
	assert.Equal(t, IDETypeVSCode, cfg.Type)
	assert.Empty(t, cfg.Version)
	assert.Equal(t, 10081, cfg.Port)
	assert.Nil(t, cfg.Options)
}

func TestIDEStatus(t *testing.T) {
	status := IDEStatus{
		Type: IDETypeVSCode, Status: "running", Port: 10081,
		PID: 12345, URL: "http://localhost:10081",
		Config: IDEConfig{Type: IDETypeVSCode, Port: 10081},
	}
	assert.Equal(t, "running", status.Status)
	assert.Equal(t, 12345, status.PID)
	assert.Equal(t, "http://localhost:10081", status.URL)
}

func setupTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestNewRunner(t *testing.T) {
	setupTestHome(t)
	runner, err := NewRunner()
	require.NoError(t, err)
	require.NotNil(t, runner)
	assert.NotEmpty(t, runner.GetWorkDir())
	assert.NotEmpty(t, runner.GetVSCodiumDir())
	assert.NotEmpty(t, runner.GetServerPath())
}

func TestRunnerIsInstalled(t *testing.T) {
	setupTestHome(t)
	runner, err := NewRunner()
	require.NoError(t, err)

	assert.False(t, runner.IsInstalled())

	os.MkdirAll(filepath.Dir(runner.GetServerPath()), 0755)
	os.WriteFile(runner.GetServerPath(), []byte("fake server"), 0755)
	os.WriteFile(filepath.Join(runner.GetVSCodiumDir(), "product.json"), []byte("{}"), 0644)

	assert.True(t, runner.IsInstalled())
}

func TestRunnerPIDManagement(t *testing.T) {
	setupTestHome(t)
	runner, err := NewRunner()
	require.NoError(t, err)

	assert.Equal(t, 0, runner.GetRunningPort())

	runner.savePID(12345, 10081)
	assert.Equal(t, 10081, runner.GetRunningPort())

	runner.removePID()
	assert.Equal(t, 0, runner.GetRunningPort())
}

func createTestTarGz(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for name, content := range entries {
		hdr := &tar.Header{
			Name: name, Mode: 0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		require.NoError(t, tarWriter.WriteHeader(hdr))
		_, err := tarWriter.Write([]byte(content))
		require.NoError(t, err)
	}
	tarWriter.Close()
	gzWriter.Close()

	tmpFile := filepath.Join(t.TempDir(), "test.tar.gz")
	err := os.WriteFile(tmpFile, buf.Bytes(), 0644)
	require.NoError(t, err)
	return tmpFile
}

func TestRunnerExtract(t *testing.T) {
	setupTestHome(t)
	entries := map[string]string{
		"vscodium-reh-web-linux-x64-1.0.0/bin/codium-server":                "fake binary content",
		"vscodium-reh-web-linux-x64-1.0.0/resources/app/out/index.js":       "fake js",
		"vscodium-reh-web-linux-x64-1.0.0/product.json":                      `{"version":"1.0.0"}`,
	}
	tarGzPath := createTestTarGz(t, entries)

	runner, err := NewRunner()
	require.NoError(t, err)

	err = runner.extract(tarGzPath)
	require.NoError(t, err)

	expectedFiles := []string{
		filepath.Join(runner.GetVSCodiumDir(), "bin", "codium-server"),
		filepath.Join(runner.GetVSCodiumDir(), "resources", "app", "out", "index.js"),
	}
	for _, f := range expectedFiles {
		assert.FileExists(t, f)
	}
}

func TestRunnerInstallFromTar(t *testing.T) {
	setupTestHome(t)
	entries := map[string]string{
		"vscodium-reh-web-linux-x64-1.0.0/bin/codium-server": "binary",
		"vscodium-reh-web-linux-x64-1.0.0/product.json":       `{"version":"1.0.0"}`,
	}
	tarGzPath := createTestTarGz(t, entries)

	runner, err := NewRunner()
	require.NoError(t, err)

	err = runner.InstallFromTar(tarGzPath, "1.0.0")
	require.NoError(t, err)
	assert.True(t, runner.IsInstalled())
}

func TestRunnerInstallFromTarIdempotent(t *testing.T) {
	setupTestHome(t)
	entries := map[string]string{
		"vscodium-reh-web-1.0.0/bin/codium-server": "binary",
		"vscodium-reh-web-1.0.0/product.json":       `{"version":"1.0.0"}`,
	}
	tarGzPath := createTestTarGz(t, entries)

	runner, err := NewRunner()
	require.NoError(t, err)

	require.NoError(t, runner.InstallFromTar(tarGzPath, "1.0.0"))
	require.NoError(t, runner.InstallFromTar(tarGzPath, "1.0.0"))
}

func TestRunnerUninstall(t *testing.T) {
	setupTestHome(t)
	entries := map[string]string{
		"vscodium-reh-web-1.0.0/bin/codium-server": "binary",
		"vscodium-reh-web-1.0.0/product.json":       `{"version":"1.0.0"}`,
	}
	tarGzPath := createTestTarGz(t, entries)

	runner, err := NewRunner()
	require.NoError(t, err)

	runner.InstallFromTar(tarGzPath, "1.0.0")
	require.NoError(t, runner.Uninstall())
	assert.False(t, runner.IsInstalled())
}

func TestRunnerStartCommand(t *testing.T) {
	setupTestHome(t)
	runner, err := NewRunner()
	require.NoError(t, err)

	cmd := runner.startCommand(10081)
	require.NotNil(t, cmd)

	expectedArgs := []string{"--port", "10081", "--host", "0.0.0.0", "--without-connection-token"}
	for i, arg := range expectedArgs {
		assert.Equal(t, arg, cmd.Args[i+1], "arg %d mismatch", i+1)
	}
}

func TestDefaultVersion(t *testing.T) {
	assert.Equal(t, "1.116.02821", DefaultVersion)
}
