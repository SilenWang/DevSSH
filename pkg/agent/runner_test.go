package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func setupTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", dir)
	return dir
}

func TestNewRunner(t *testing.T) {
	setupTestHome(t)
	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}
	if runner == nil {
		t.Fatal("expected non-nil runner")
	}
	if runner.GetWorkDir() == "" {
		t.Error("expected non-empty WorkDir")
	}
	if runner.GetVSCodiumDir() == "" {
		t.Error("expected non-empty VSCodiumDir")
	}
	if runner.GetServerPath() == "" {
		t.Error("expected non-empty ServerPath")
	}
}

func TestRunnerIsInstalled(t *testing.T) {
	setupTestHome(t)
	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	if runner.IsInstalled() {
		t.Error("expected not installed initially")
	}

	os.MkdirAll(filepath.Dir(runner.GetServerPath()), 0755)
	os.WriteFile(runner.GetServerPath(), []byte("fake server"), 0755)

	if !runner.IsInstalled() {
		t.Error("expected installed after creating server file")
	}
}

func TestRunnerPIDManagement(t *testing.T) {
	setupTestHome(t)
	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	port := runner.GetRunningPort()
	if port != 0 {
		t.Errorf("expected port 0 initially, got %d", port)
	}

	runner.savePID(12345, 10081)

	loadedPort := runner.GetRunningPort()
	if loadedPort != 10081 {
		t.Errorf("expected port 10081, got %d", loadedPort)
	}

	runner.removePID()

	portAfterRemove := runner.GetRunningPort()
	if portAfterRemove != 0 {
		t.Errorf("expected port 0 after removing PID, got %d", portAfterRemove)
	}
}

func createTestTarGz(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for name, content := range entries {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tarWriter.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write tar content: %v", err)
		}
	}

	tarWriter.Close()
	gzWriter.Close()

	tmpFile := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := os.WriteFile(tmpFile, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write test tar.gz: %v", err)
	}
	return tmpFile
}

func TestRunnerExtract(t *testing.T) {
	setupTestHome(t)

	// Create a test tar.gz that mimics VSCodium structure
	entries := map[string]string{
		"vscodium-reh-web-linux-x64-1.0.0/bin/codium-server": "fake binary content",
		"vscodium-reh-web-linux-x64-1.0.0/resources/app/out/index.js": "fake js content",
	}
	tarGzPath := createTestTarGz(t, entries)

	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	if err := runner.extract(tarGzPath); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	// Check that files were extracted with prefix stripping
	expectedFiles := []string{
		filepath.Join(runner.GetVSCodiumDir(), "bin", "codium-server"),
		filepath.Join(runner.GetVSCodiumDir(), "resources", "app", "out", "index.js"),
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist after extraction", f)
		}
	}
}

func TestRunnerInstallFromTar(t *testing.T) {
	setupTestHome(t)
	entries := map[string]string{
		"vscodium-reh-web-linux-x64-1.0.0/bin/codium-server": "binary",
	}
	tarGzPath := createTestTarGz(t, entries)

	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	if err := runner.InstallFromTar(tarGzPath, "1.0.0"); err != nil {
		t.Fatalf("InstallFromTar failed: %v", err)
	}

	if !runner.IsInstalled() {
		t.Error("expected installed after InstallFromTar")
	}
}

func TestRunnerInstallFromTarIdempotent(t *testing.T) {
	setupTestHome(t)
	entries := map[string]string{
		"vscodium-reh-web-1.0.0/bin/codium-server": "binary",
	}
	tarGzPath := createTestTarGz(t, entries)

	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	if err := runner.InstallFromTar(tarGzPath, "1.0.0"); err != nil {
		t.Fatalf("first InstallFromTar failed: %v", err)
	}

	// Second call should be idempotent
	if err := runner.InstallFromTar(tarGzPath, "1.0.0"); err != nil {
		t.Fatalf("second InstallFromTar failed: %v", err)
	}
}

func TestRunnerUninstall(t *testing.T) {
	setupTestHome(t)
	entries := map[string]string{
		"vscodium-reh-web-1.0.0/bin/codium-server": "binary",
	}
	tarGzPath := createTestTarGz(t, entries)

	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	runner.InstallFromTar(tarGzPath, "1.0.0")

	if err := runner.Uninstall(); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	if runner.IsInstalled() {
		t.Error("expected not installed after uninstall")
	}
}

func TestRunnerStartCommand(t *testing.T) {
	setupTestHome(t)
	runner, err := NewRunner()
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	cmd := runner.startCommand(10081)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	expectedArgs := []string{"--port", "10081", "--host", "0.0.0.0", "--without-connection-token"}
	for i, arg := range expectedArgs {
		if cmd.Args[i+1] != arg {
			t.Errorf("expected arg %d = %q, got %q", i+1, arg, cmd.Args[i+1])
		}
	}
}

func TestDefaultVersion(t *testing.T) {
		if DefaultVersion != "1.116.02821" {
		t.Errorf("expected DefaultVersion 1.121.03429, got %s", DefaultVersion)
	}
}
