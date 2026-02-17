package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"devssh/pkg/config"
	"devssh/pkg/download"
	"devssh/pkg/ssh"
	"devssh/pkg/tunnel"

	"github.com/loft-sh/log"
)

func uploadToRemote(client *ssh.Client, localPath, remotePath string) error {
	scpClient := ssh.NewSCPClient(client)
	return scpClient.Upload(localPath, remotePath)
}

func checkRemoteDevSSH(client *ssh.Client) (exists bool, version string, err error) {
	cmd := "test -f ~/.devssh/bin/devssh && ~/.devssh/bin/devssh --version 2>/dev/null || echo 'not_found'"
	output, err := client.RunCommand(cmd)
	if err != nil {
		return false, "", nil
	}
	if strings.Contains(output, "not_found") {
		return false, "", nil
	}
	version = strings.TrimSpace(output)
	version = strings.TrimPrefix(version, "devssh version ")
	return true, version, nil
}

func detectRemoteArch(client *ssh.Client) (os, arch string, err error) {
	osCmd := "uname -s"
	osOutput, err := client.RunCommand(osCmd)
	if err != nil {
		return "", "", fmt.Errorf("failed to detect OS: %w", err)
	}
	os = strings.ToLower(strings.TrimSpace(osOutput))

	archCmd := "uname -m"
	archOutput, err := client.RunCommand(archCmd)
	if err != nil {
		return "", "", fmt.Errorf("failed to detect architecture: %w", err)
	}
	arch = strings.ToLower(strings.TrimSpace(archOutput))

	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	case "armv7l", "armv8l":
		arch = "arm"
	}

	return os, arch, nil
}

func deployDevSSH(client *ssh.Client, version string, logger log.Logger) error {
	remoteOS, remoteArch, err := detectRemoteArch(client)
	if err != nil {
		return fmt.Errorf("failed to detect remote arch: %w", err)
	}

	if remoteOS != "linux" {
		return fmt.Errorf("unsupported platform: %s. Only Linux is supported", remoteOS)
	}

	if remoteArch != "amd64" && remoteArch != "arm64" {
		return fmt.Errorf("unsupported architecture: %s. Only amd64 and arm64 are supported", remoteArch)
	}

	url := config.GetDevSSHDownloadURL(version, remoteOS, remoteArch)

	logger.Infof("Downloading devssh %s for %s/%s from %s ...", version, remoteOS, remoteArch, url)

	cacheDir, err := getCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}

	downloader := download.NewLocalDownloader(cacheDir, logger)
	localPath, err := downloader.Download(url)
	if err != nil {
		return fmt.Errorf("failed to download devssh: %w", err)
	}
	defer os.Remove(localPath)

	logger.Infof("Uploading devssh to remote...")
	if err := uploadToRemote(client, localPath, "~/.devssh/bin/devssh"); err != nil {
		return fmt.Errorf("failed to upload devssh: %w", err)
	}

	logger.Infof("Setting executable permissions...")
	client.RunCommand("chmod +x ~/.devssh/bin/devssh")

	return nil
}

func runRemoteAgentCommand(client *ssh.Client, args string) (string, error) {
	cmd := fmt.Sprintf("~/.devssh/bin/devssh agent %s", args)
	output, err := client.RunCommand(cmd)
	if err != nil {
		return output, fmt.Errorf("failed to run agent command: %w, output: %s", err, output)
	}
	return output, nil
}

func downloadVSCodeLocal(version, os, arch string, logger log.Logger) (string, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get cache directory: %w", err)
	}

	downloader := download.NewLocalDownloader(cacheDir, logger)
	return downloader.DownloadVSCode(version, os, arch)
}

func getCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(homeDir, ".cache", "devssh")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}
	return cacheDir, nil
}

func doUpCommand(client *ssh.Client, host string, ideType string, idePort int, version string, forwards []string, auto bool, logger log.Logger) error {
	logger.Infof("Detecting remote platform...")
	remoteOS, remoteArch, err := detectRemoteArch(client)
	if err != nil {
		return fmt.Errorf("failed to detect remote platform: %w", err)
	}

	if remoteOS != "linux" {
		return fmt.Errorf("unsupported platform: %s. Only Linux is supported", remoteOS)
	}

	if remoteArch != "amd64" && remoteArch != "arm64" {
		return fmt.Errorf("unsupported architecture: %s. Only amd64 and arm64 are supported", remoteArch)
	}

	logger.Infof("Target platform: %s/%s", remoteOS, remoteArch)

	logger.Infof("Checking devssh on remote...")
	exists, remoteVersion, _ := checkRemoteDevSSH(client)
	if !exists || remoteVersion != GetVersion() {
		logger.Infof("Deploying devssh %s...", GetVersion())
		if err := deployDevSSH(client, GetVersion(), logger); err != nil {
			return fmt.Errorf("failed to deploy devssh: %w", err)
		}
	} else {
		logger.Infof("devssh %s is already installed", remoteVersion)
	}

	logger.Infof("Checking VSCode installation on remote...")
	vscodeCheckCmd := "test -f ~/.devssh/openvscode/bin/openvscode-server && echo 'installed' || echo 'not_installed'"
	checkOutput, checkErr := client.RunCommand(vscodeCheckCmd)
	if checkErr != nil {
		return fmt.Errorf("failed to check remote VSCode: %w", checkErr)
	}

	if strings.Contains(checkOutput, "not_installed") {
		logger.Infof("Downloading VSCode %s for %s/%s...", version, remoteOS, remoteArch)
		vscodePath, err := downloadVSCodeLocal(version, remoteOS, remoteArch, logger)
		if err != nil {
			return fmt.Errorf("failed to download VSCode: %w", err)
		}

		logger.Infof("Uploading VSCode to remote...")
		if err := uploadToRemote(client, vscodePath, "~/.devssh/openvscode.tar.gz"); err != nil {
			return fmt.Errorf("failed to upload VSCode: %w", err)
		}

		logger.Infof("Installing VSCode on remote...")
		if _, err := runRemoteAgentCommand(client, "install --local-tar ~/.devssh/openvscode.tar.gz"); err != nil {
			return fmt.Errorf("failed to install VSCode: %w", err)
		}
	} else {
		logger.Infof("VSCode is already installed, skipping installation")
	}

	logger.Infof("Checking VSCode status on remote...")
	currentPort := 0
	isRunning := false

	vscodeCheckCmdStr := "~/.devssh/bin/devssh agent is-running"
	checkOutput, cmdErr := client.RunCommand(vscodeCheckCmdStr)
	if cmdErr == nil && strings.Contains(checkOutput, "running") {
		isRunning = true
	}

	if isRunning {
		vscodePortCmdStr := "~/.devssh/bin/devssh agent get-port"
		portOutput, cmdErr := client.RunCommand(vscodePortCmdStr)
		if cmdErr == nil {
			portStr := strings.TrimSpace(portOutput)
			currentPort, _ = strconv.Atoi(portStr)
		}
	}

	shouldStart := false
	if !isRunning {
		logger.Infof("VSCode is not running, will start it")
		shouldStart = true
	} else if currentPort == 0 {
		logger.Infof("VSCode is running but port is unknown, restarting with specified port %d...", idePort)
		logger.Infof("Stopping VSCode...")
		_, cmdErr := client.RunCommand("~/.devssh/bin/devssh agent stop")
		if cmdErr != nil {
			pidCmd := "cat ~/.devssh/agent.pid | grep pid= | cut -d= -f2"
			pidOutput, _ := client.RunCommand(pidCmd)
			return fmt.Errorf("failed to stop VSCode (pid: %s). Please manually kill the process and try again", strings.TrimSpace(pidOutput))
		}
		shouldStart = true
	} else if currentPort > 0 && currentPort != idePort {
		logger.Warnf("VSCode is running on port %d, but requested port is %d", currentPort, idePort)
		logger.Infof("Stopping VSCode...")
		_, cmdErr := client.RunCommand("~/.devssh/bin/devssh agent stop")
		if cmdErr != nil {
			pidCmd := "cat ~/.devssh/agent.pid | grep pid= | cut -d= -f2"
			pidOutput, _ := client.RunCommand(pidCmd)
			return fmt.Errorf("failed to stop VSCode (pid: %s). Please manually kill the process and try again", strings.TrimSpace(pidOutput))
		}
		shouldStart = true
	} else {
		logger.Infof("VSCode is already running on port %d, skipping start", currentPort)
	}

	if shouldStart {
		logger.Infof("Starting VSCode on port %d...", idePort)
		if _, err := runRemoteAgentCommand(client, fmt.Sprintf("start --port %d", idePort)); err != nil {
			return fmt.Errorf("failed to start VSCode: %w", err)
		}
	}

	tunnelManager := tunnel.NewTunnelManagerWithLogger(logger)

	var forwardConfigs []tunnel.ForwardConfig
	if auto {
		forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{AutoDetect: true})
	} else {
		for _, forward := range forwards {
			parts := strings.Split(forward, ":")
			if len(parts) == 1 {
				port, err := strconv.Atoi(parts[0])
				if err != nil {
					return fmt.Errorf("invalid port: %s", parts[0])
				}
				forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{
					LocalPort:  port,
					RemotePort: port,
				})
			} else if len(parts) == 2 {
				localPort, err := strconv.Atoi(parts[0])
				if err != nil {
					return fmt.Errorf("invalid local port: %s", parts[0])
				}
				remotePort, err := strconv.Atoi(parts[1])
				if err != nil {
					return fmt.Errorf("invalid remote port: %s", parts[1])
				}
				forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{
					LocalPort:  localPort,
					RemotePort: remotePort,
				})
			}
		}
		forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{
			LocalPort:  idePort,
			RemotePort: idePort,
		})
	}

	portResults, err := tunnel.CreatePortForwards(client, forwardConfigs, tunnelManager)
	if err != nil {
		return fmt.Errorf("failed to create port forwards: %w", err)
	}

	tunnels := tunnelManager.ListTunnels()
	logger.Infof("Active port forwards:")
	for name, info := range tunnels {
		logger.Infof("  %s: localhost:%d -> remote:%d", name, info.LocalPort, info.RemotePort)
	}

	actualIDEPort := idePort
	for _, result := range portResults {
		if result.RemotePort == idePort {
			actualIDEPort = result.ActualPort
			break
		}
	}
	if actualIDEPort == idePort {
		for _, info := range tunnels {
			if info.RemotePort == idePort {
				actualIDEPort = info.LocalPort
				break
			}
		}
	}

	logger.Infof("%s is now accessible at http://localhost:%d", ideType, actualIDEPort)
	logger.Infof("Press Ctrl+C to stop...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigChan:
	}

	return nil
}
