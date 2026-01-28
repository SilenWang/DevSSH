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
	cmd := "test -x ~/.devssh/bin/devssh && ~/.devssh/bin/devssh --version 2>/dev/null || echo 'not_found'"
	output, err := client.RunCommand(cmd)
	if err != nil {
		return false, "", nil
	}
	if strings.Contains(output, "not_found") {
		return false, "", nil
	}
	version = strings.TrimSpace(output)
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

	url := config.GetDevSSHDownloadURL(version, remoteOS, remoteArch)

	logger.Infof("Downloading devssh %s for %s/%s...", version, remoteOS, remoteArch)

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

func downloadVSCodeLocal(version string, logger log.Logger) (string, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get cache directory: %w", err)
	}

	downloader := download.NewLocalDownloader(cacheDir, logger)
	return downloader.DownloadVSCode(version)
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

	logger.Infof("Downloading VSCode %s...", version)
	vscodePath, err := downloadVSCodeLocal(version, logger)
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

	logger.Infof("Starting VSCode on port %d...", idePort)
	if _, err := runRemoteAgentCommand(client, fmt.Sprintf("start --port %d", idePort)); err != nil {
		return fmt.Errorf("failed to start VSCode: %w", err)
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
