//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type DevSSHIntegrationSuite struct {
	suite.Suite
	binaryPath string
	dockerName string
	sshPort    string
	httpPort   string
	artifactDir string
	httpCmd    *exec.Cmd
}

func findProjectRoot() string {
	wd, _ := os.Getwd()
	for dir := wd; dir != "/"; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	return wd
}

func checkPrerequisite(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func TestDevSSHIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(DevSSHIntegrationSuite))
}

func (s *DevSSHIntegrationSuite) SetupSuite() {
	required := []string{"docker", "python3", "sshpass", "nc"}
	for _, bin := range required {
		if !checkPrerequisite(bin) {
			s.T().Fatalf("prerequisite not found: %s (install with: sudo apt-get install -y %s)", bin, map[string]string{
				"docker":  "docker.io",
				"python3": "python3",
				"sshpass": "sshpass",
				"nc":      "netcat-openbsd",
			}[bin])
		}
	}

	s.sshPort = "10022"
	s.httpPort = "19999"
	s.dockerName = fmt.Sprintf("devssh-test-%d", time.Now().UnixNano())

	projectRoot := findProjectRoot()

	s.T().Log("=== Building devssh binary ===")
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(projectRoot, "bin", "devssh-linux-amd64"), "./cmd/devssh/")
	buildCmd.Dir = projectRoot
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := buildCmd.CombinedOutput()
	s.Require().NoError(err, "build failed: %s", string(output))

	s.binaryPath = filepath.Join(projectRoot, "bin", "devssh-linux-amd64")
	s.Require().FileExists(s.binaryPath)

	s.T().Log("=== Creating test artifacts ===")
	s.artifactDir = s.T().TempDir()

	vscodeDir := filepath.Join(s.artifactDir, "vscodium-reh-web-linux-x64-1.116.02821", "bin")
	os.MkdirAll(vscodeDir, 0755)
	fakeScript := filepath.Join(vscodeDir, "codium-server")
	fakeContent := []byte(fmt.Sprintf(`#!/bin/bash
PORT=$2
echo "Fake codium-server listening on port $PORT"
while true; do
    echo -e "HTTP/1.1 200 OK\r\n\r\nFake VSCode" | nc -l "$PORT" 2>/dev/null
done
`))
	err = os.WriteFile(fakeScript, fakeContent, 0755)
	s.Require().NoError(err)

	vscodeTarGz := filepath.Join(s.artifactDir, "vscodium-reh-web-linux-x64-1.116.02821.tar.gz")
	tarCmd := exec.Command("tar", "-czf", vscodeTarGz, "-C", s.artifactDir, "vscodium-reh-web-linux-x64-1.116.02821")
	s.Require().NoError(tarCmd.Run())

	s.T().Log("=== Starting HTTP file server ===")
	s.httpCmd = exec.Command("python3", "-m", "http.server", s.httpPort, "--directory", s.artifactDir)
	s.httpCmd.Stdout = &bytes.Buffer{}
	s.httpCmd.Stderr = &bytes.Buffer{}
	s.Require().NoError(s.httpCmd.Start())
	time.Sleep(1 * time.Second)

	s.T().Log("=== Starting Docker SSH container ===")
	dockerCmd := exec.Command("docker", "run", "-d",
		"--name", s.dockerName,
		"-p", fmt.Sprintf("%s:22", s.sshPort),
		"-e", "USER=testuser",
		"-e", "PASSWORD=testpass",
		"-e", "SUDO_ACCESS=true",
		"-e", "PASSWORD_ACCESS=true",
		"lscr.io/linuxserver/openssh-server:latest")
	output, err = dockerCmd.CombinedOutput()
	s.Require().NoError(err, "docker run failed: %s", string(output))

	s.T().Log("Waiting for SSH server (TCP)...")
	for i := 0; i < 30; i++ {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+s.sshPort, 2*time.Second)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(2 * time.Second)
	}

	s.T().Log("Waiting for SSH server (SSH handshake)...")
	for i := 0; i < 45; i++ {
		sshCheck := exec.Command("sshpass", "-p", "testpass", "ssh",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "ConnectTimeout=5",
			"-p", s.sshPort,
			"testuser@127.0.0.1",
			"echo READY")
		output, err := sshCheck.CombinedOutput()
		if err == nil && strings.Contains(string(output), "READY") {
			s.T().Log("SSH server is ready")
			break
		}
		if i == 44 {
			// Dump docker logs before failing
			logsCmd := exec.Command("docker", "logs", s.dockerName)
			logs, _ := logsCmd.CombinedOutput()
			s.T().Fatalf("SSH server not ready after 45 attempts, last output: %s\nDocker logs:\n%s", string(output), string(logs))
		}
		time.Sleep(2 * time.Second)
	}

	s.copyBinaryToArtifacts()
}

func (s *DevSSHIntegrationSuite) copyBinaryToArtifacts() {
	devsshArtifact := filepath.Join(s.artifactDir, "devssh-0.1.8-linux-amd64")
	input, err := os.ReadFile(s.binaryPath)
	s.Require().NoError(err)
	err = os.WriteFile(devsshArtifact, input, 0755)
	s.Require().NoError(err)
}

func (s *DevSSHIntegrationSuite) TearDownSuite() {
	s.T().Log("=== Cleaning up ===")
	// Dump docker logs before cleanup if test failed
	if s.T().Failed() {
		logsCmd := exec.Command("docker", "logs", s.dockerName)
		logs, _ := logsCmd.CombinedOutput()
		s.T().Logf("Docker container logs:\n%s", string(logs))
	}
	if s.httpCmd != nil && s.httpCmd.Process != nil {
		s.httpCmd.Process.Kill()
	}
	dockerCmd := exec.Command("docker", "rm", "-f", s.dockerName)
	dockerCmd.Run()
}

func (s *DevSSHIntegrationSuite) TestUpCommand() {
	t := s.T()
	require := s.Require()

	t.Log("=== Running devssh up ===")
	cmd := exec.Command(s.binaryPath, "up",
		"-u", "testuser",
		"-p", s.sshPort,
		"--password", "testpass",
		"--timeout", "30",
		"--keepalive=false",
		"127.0.0.1")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("DEVSSH_DEVSSH_DOWNLOAD_URL=http://localhost:%s/devssh-{{version}}-{{os}}-{{arch}}", s.httpPort),
		fmt.Sprintf("DEVSSH_VSCODE_DOWNLOAD_URL=http://localhost:%s/vscodium-reh-web-{{os}}-{{arch}}-{{version}}.tar.gz", s.httpPort),
		fmt.Sprintf("DEVSSH_LOCAL_BINARY_PATH=%s", s.binaryPath),
	)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)
	t.Logf("devssh up output:\n%s", outputStr)

	require.NoError(err, "devssh up failed: %s", outputStr)
	require.Contains(outputStr, "accessible at", "should show accessible URL")

	var localPort int
	for _, line := range strings.Split(outputStr, "\n") {
		if strings.Contains(line, "accessible at") {
			parts := strings.Split(line, ":")
			if len(parts) > 0 {
				portStr := strings.TrimSpace(parts[len(parts)-1])
				localPort, _ = strconv.Atoi(portStr)
			}
		}
	}
	require.NotZero(localPort, "should parse local port from output")

	t.Logf("Verifying VSCode is accessible at http://localhost:%d", localPort)
	for i := 0; i < 10; i++ {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d", localPort))
		if err == nil {
			resp.Body.Close()
			require.Equal(http.StatusOK, resp.StatusCode)
			return
		}
		time.Sleep(1 * time.Second)
	}
	t.Error("VSCode not accessible after 10 seconds")
}

func (s *DevSSHIntegrationSuite) TestSSHConnection() {
	cmd := exec.Command("sshpass", "-p", "testpass", "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-p", s.sshPort,
		"testuser@127.0.0.1",
		"echo 'SSH_OK'")
	output, err := cmd.CombinedOutput()
	s.Require().NoError(err, "ssh failed: %s", string(output))
	s.Require().Contains(string(output), "SSH_OK")
}
