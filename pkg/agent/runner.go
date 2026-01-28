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
	"syscall"
	"time"
)

const (
	DefaultVersion = "v1.105.1"
)

type Runner struct {
	workDir    string
	logFile    string
	binDir     string
	serverPath string
	serverPID  int
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
	binDir := filepath.Join(workDir, "bin")

	return &Runner{
		workDir:    workDir,
		logFile:    logFile,
		binDir:     binDir,
		serverPath: filepath.Join(binDir, "openvscode-server"),
	}, nil
}

func (r *Runner) Install(version string) error {
	if version == "" {
		version = DefaultVersion
	}

	if r.IsInstalled() {
		return nil
	}

	fmt.Println("Downloading openvscode-server...")

	url := r.getDownloadURL(version)
	downloadPath := filepath.Join(r.workDir, fmt.Sprintf("openvscode-server-%s.tar.gz", version))

	if err := r.download(url, downloadPath); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	fmt.Println("Extracting...")

	if err := os.MkdirAll(r.binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	if err := r.extract(downloadPath); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	os.Remove(downloadPath)

	return nil
}

func (r *Runner) Start(port int) error {
	if !r.IsInstalled() {
		return fmt.Errorf("VSCode is not installed. Run 'devssh agent install' first")
	}

	if r.IsRunning() {
		return nil
	}

	if port == 0 {
		port = 10080
	}

	cmd := r.startCommand(port)

	logFile, err := os.OpenFile(r.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	r.serverPID = cmd.Process.Pid

	r.savePID(r.serverPID)

	fmt.Printf("VSCode started with PID %d\n", r.serverPID)

	return nil
}

func (r *Runner) Stop() error {
	if !r.IsRunning() {
		return nil
	}

	proc, err := os.FindProcess(r.serverPID)
	if err != nil {
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		proc.Kill()
	}

	time.Sleep(time.Second)

	r.removePID()

	fmt.Println("VSCode stopped")

	return nil
}

func (r *Runner) IsInstalled() bool {
	_, err := os.Stat(r.serverPath)
	return err == nil
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

func (r *Runner) getDownloadURL(version string) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	baseURL := fmt.Sprintf("https://github.com/gitpod-io/openvscode-server/releases/download/%s/openvscode-server-%s", version, version)

	switch os {
	case "linux":
		if arch == "amd64" {
			return baseURL + "-linux-x64.tar.gz"
		} else if arch == "arm64" {
			return baseURL + "-linux-arm64.tar.gz"
		}
	case "darwin":
		if arch == "amd64" {
			return baseURL + "-darwin-x64.tar.gz"
		} else if arch == "arm64" {
			return baseURL + "-darwin-arm64.tar.gz"
		}
	}

	return baseURL + fmt.Sprintf("-%s-%s.tar.gz", os, arch)
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

		targetPath := filepath.Join(r.binDir, header.Name)

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
			if !filepath.IsAbs(linkTarget) {
				linkTarget = filepath.Join(filepath.Dir(targetPath), linkTarget)
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

func (r *Runner) savePID(pid int) {
	pidPath := r.pidPath()
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0644)
}

func (r *Runner) loadPID() {
	pidPath := r.pidPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	fmt.Sscanf(string(data), "%d", &r.serverPID)
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
