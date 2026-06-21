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

func TestDevSSHIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(DevSSHIntegrationSuite))
}

func (s *DevSSHIntegrationSuite) SetupSuite() {
	s.sshPort = "10022"
	s.httpPort = "19999"
	s.dockerName = fmt.Sprintf("devssh-test-%d", time.Now().UnixNano())

	s.T().Log("=== Building devssh binary ===")
	buildCmd := exec.Command("go", "build", "-o", "bin/devssh-linux-amd64", "./cmd/devssh/")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := buildCmd.CombinedOutput()
	s.Require().NoError(err, "build failed: %s", string(output))

	wd, err := os.Getwd()
	s.Require().NoError(err)
	s.binaryPath = filepath.Join(wd, "bin", "devssh-linux-amd64")
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
    echo -e "HTTP/1.1 200 OK\r\n\r\nFake VSCode" | nc -l -p "$PORT" -q 1 2>/dev/null
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
		"-e", "SSH_USER=testuser",
		"-e", "SSH_PASSWORD=testpass",
		"lscr.io/linuxserver/openssh-server:latest")
	output, err = dockerCmd.CombinedOutput()
	s.Require().NoError(err, "docker run failed: %s", string(output))

	s.T().Log("Waiting for SSH server...")
	for i := 0; i < 30; i++ {
		conn, err := net.DialTimeout("tcp", "localhost:"+s.sshPort, 2*time.Second)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(1 * time.Second)
	}
	time.Sleep(3 * time.Second)

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
		"localhost")
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
		"testuser@localhost",
		"echo 'SSH_OK'")
	output, err := cmd.CombinedOutput()
	s.Require().NoError(err, "ssh failed: %s", string(output))
	s.Require().Contains(string(output), "SSH_OK")
}
