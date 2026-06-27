package agent

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"devssh/pkg/download"
	"devssh/pkg/logging"
)

const (
	DefaultVersion = "1.116.02821"
)

type Runner struct {
	workDir        string
	logFile        string
	vscodiumDir    string
	serverPath     string
	versionFile    string
	serverPID      int
}

func NewRunner() (*Runner, error) {
	homeDir, err := getHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	workDir := filepath.Join(homeDir, ".devssh")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	logFile := filepath.Join(workDir, "agent.log")
	vscodiumDir := filepath.Join(workDir, "vscodium")

	return &Runner{
		workDir:     workDir,
		logFile:     logFile,
		vscodiumDir: vscodiumDir,
		serverPath:  filepath.Join(vscodiumDir, "bin", "codium-server"),
		versionFile: filepath.Join(vscodiumDir, "version"),
	}, nil
}

func (r *Runner) Install(version string) error {
	if version == "" {
		version = DefaultVersion
	}

	if r.IsInstalled() {
		return nil
	}

	logging.Infof("Downloading VSCodium reh-web...")

	url := download.GetVSCodeDownloadURL(version, runtime.GOOS, runtime.GOARCH)
	logging.Infof("%s", url)
	downloadPath := filepath.Join(r.workDir, fmt.Sprintf("vscodium-reh-web-%s.tar.gz", version))

	if err := r.download(url, downloadPath); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	logging.Infof("Extracting...")

	if err := os.MkdirAll(r.vscodiumDir, 0755); err != nil {
		return fmt.Errorf("failed to create vscodium directory: %w", err)
	}

	if err := r.extract(downloadPath); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	os.Remove(downloadPath)

	if err := r.saveVersion(version); err != nil {
		logging.Warnf("Failed to save version file: %v", err)
	}

	return nil
}

func (r *Runner) saveVersion(version string) error {
	return os.WriteFile(r.versionFile, []byte(version), 0644)
}

func (r *Runner) GetInstalledVersion() string {
	data, err := os.ReadFile(r.versionFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (r *Runner) Start(port int) error {
	if !r.IsInstalled() {
		return fmt.Errorf("VSCode is not installed. Run 'devssh agent install' first")
	}

	if r.IsRunning() {
		return nil
	}

	if port == 0 {
		port = 10081
	}

	cmd := r.startCommand(port)

	logFile, err := os.OpenFile(r.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	r.serverPID = cmd.Process.Pid

	r.savePID(r.serverPID, port)

	logging.Infof("VSCode started with PID %d", r.serverPID)

	return nil
}

func (r *Runner) Stop() error {
	if !r.IsRunning() {
		return nil
	}

	pgid, err := syscall.Getpgid(r.serverPID)
	if err != nil {
		r.removePID()
		return nil
	}

	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
	}

	time.Sleep(time.Second)

	r.removePID()

	logging.Infof("VSCode stopped")

	return nil
}

func (r *Runner) IsInstalled() bool {
	if _, err := os.Stat(r.serverPath); err != nil {
		return false
	}
	if _, err := os.Stat(r.versionFile); err != nil {
		return false
	}
	return true
}

func (r *Runner) IsRunning() bool {
	if r.serverPID == 0 {
		r.loadPID()
	}

	if r.serverPID == 0 {
		return false
	}

	err := syscall.Kill(r.serverPID, 0)
	return err == nil
}

func (r *Runner) download(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func (r *Runner) extract(archivePath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		name := header.Name

		firstComponent := strings.Split(name, "/")[0]

		if strings.HasPrefix(name, "vscodium-reh-web-") && strings.Contains(name, "/") {
			parts := strings.SplitN(name, "/", 2)
			if len(parts) > 1 {
				name = parts[1]
			}
		}

		if name == "" || name == firstComponent {
			continue
		}

		targetPath := filepath.Join(r.vscodiumDir, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			dir := filepath.Dir(targetPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			outFile.Close()
		case tar.TypeSymlink:
			linkTarget := header.Linkname

			if strings.HasPrefix(linkTarget, "vscodium-reh-web-") {
				parts := strings.SplitN(linkTarget, "/", 2)
				if len(parts) > 1 {
					linkTarget = parts[1]
				}
			}

			if !filepath.IsAbs(linkTarget) {
				linkTarget = filepath.Join(r.vscodiumDir, linkTarget)
			}

			dir := filepath.Dir(targetPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			if err := os.Symlink(linkTarget, targetPath); err != nil {
				if !os.IsExist(err) {
					return fmt.Errorf("failed to create symlink: %w", err)
				}
			}
		}
	}

	return nil
}

func (r *Runner) startCommand(port int) *exec.Cmd {
	return exec.Command(r.serverPath,
		"--port", fmt.Sprintf("%d", port),
		"--host", "0.0.0.0",
		"--without-connection-token",
	)
}

func (r *Runner) savePID(pid, port int) {
	pidPath := r.pidPath()
	content := fmt.Sprintf("[lock]\npid=%d\nport=%d\n", pid, port)
	os.WriteFile(pidPath, []byte(content), 0644)
}

func (r *Runner) loadPID() {
	pidPath := r.pidPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "pid=") {
			fmt.Sscanf(line, "pid=%d", &r.serverPID)
		}
	}
}

func (r *Runner) GetRunningPort() int {
	if r.serverPID == 0 {
		r.loadPID()
	}
	if r.serverPID == 0 {
		return 0
	}
	data, err := os.ReadFile(r.pidPath())
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "port=") {
			var port int
			fmt.Sscanf(line, "port=%d", &port)
			return port
		}
	}
	return 0
}

func (r *Runner) removePID() {
	pidPath := r.pidPath()
	os.Remove(pidPath)
}

func (r *Runner) pidPath() string {
	return filepath.Join(r.workDir, "agent.pid")
}

func getHomeDir() (string, error) {
	home := os.Getenv("HOME")
	if home != "" {
		return home, nil
	}

	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	return usr.HomeDir, nil
}

func (r *Runner) InstallFromTar(tarPath string, version string) error {
	if r.IsInstalled() {
		logging.Infof("VSCode is already installed")
		return nil
	}

	logging.Infof("Installing VSCodium from local tar.gz...")

	if err := os.MkdirAll(r.vscodiumDir, 0755); err != nil {
		return fmt.Errorf("failed to create vscodium directory: %w", err)
	}

	if err := r.extract(tarPath); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	if version != "" {
		if err := r.saveVersion(version); err != nil {
			logging.Warnf("Failed to save version file: %v", err)
		}
	}

	logging.Infof("VSCode installed successfully")
	return nil
}

func (r *Runner) Uninstall() error {
	if r.IsRunning() {
		if err := r.Stop(); err != nil {
			return fmt.Errorf("failed to stop VSCode: %w", err)
		}
	}

	if r.IsInstalled() {
		if err := os.RemoveAll(r.vscodiumDir); err != nil {
			return fmt.Errorf("failed to remove vscodium directory: %w", err)
		}
	}

	r.removePID()

	logging.Infof("VSCode uninstalled successfully")
	return nil
}

func (r *Runner) GetWorkDir() string {
	return r.workDir
}

func (r *Runner) GetVSCodiumDir() string {
	return r.vscodiumDir
}

func (r *Runner) GetServerPath() string {
	return r.serverPath
}
