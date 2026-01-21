// DevSSH - SSH-based remote development environment setup
// Copyright (c) 2025 The DevSSH Authors
// Licensed under the Mozilla Public License 2.0
// See https://www.mozilla.org/en-US/MPL/2.0/ for details.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"devssh/pkg/ide"
	"devssh/pkg/logging"
	"devssh/pkg/ssh"
	"devssh/pkg/tunnel"

	"github.com/loft-sh/log"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.1"
	logger  log.Logger
)

func main() {
	// 初始化日志系统（默认使用info级别）
	logger = logging.InitDefault()

	rootCmd := &cobra.Command{
		Use:     "devssh",
		Short:   "DevSSH - SSH-based remote development environment setup",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// 处理全局标志
			verbose, _ := cmd.Flags().GetBool("verbose")
			quiet, _ := cmd.Flags().GetBool("quiet")

			// 根据标志设置日志级别
			if verbose {
				logger = logging.InitDebug()
			} else if quiet {
				logger = logging.InitQuiet()
			}

			// 设置全局logger
			logging.SetGlobalLogger(logger)
		},
	}

	// 添加全局标志
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "启用详细输出（调试级别）")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "安静模式（只显示错误）")

	// 禁用自动生成的completion命令
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.AddCommand(
		newConnectCmd(),
		newForwardCmd(),
		newListCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		logger.Errorf("%v", err)
		os.Exit(1)
	}
}

func newConnectCmd() *cobra.Command {
	var (
		user     string
		port     string
		keyPath  string
		password string
		ideType  string
		forwards []string
		auto     bool
		timeout  int
	)

	cmd := &cobra.Command{
		Use:   "connect [host]",
		Short: "Connect to remote host and setup development environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 获取logger
			logger := logging.GetGlobalLogger()
			host := args[0]

			var client *ssh.Client
			var err error

			// 检查是否是SSH配置文件中的主机
			parser := ssh.NewSSHConfigParser()
			_, sshErr := parser.GetHost(host)
			if sshErr == nil {
				// 从SSH配置文件创建客户端，使用命令行参数覆盖
				overrideConfig := &ssh.Config{
					Host: host,

					Username: user,
					KeyPath:  keyPath,
					Password: password,
					Timeout:  time.Duration(timeout) * time.Second,
				}
				// 只有当用户显式提供了-p参数时才覆盖端口
				if port != "22" {
					overrideConfig.Port = port
				}
				client, err = ssh.NewClientFromSSHConfigWithLogger(host, overrideConfig, logger)
				if err != nil {
					return fmt.Errorf("failed to create client from SSH config: %w", err)
				}
			} else {
				// 检查是否是特殊主机模式的错误
				if strings.Contains(sshErr.Error(), "is a special pattern") {
					return fmt.Errorf("cannot connect to %s: %v", host, sshErr)
				}

				// 如果不是SSH配置文件中的主机，使用传统方式
				// Parse host if it contains user@host format
				if strings.Contains(host, "@") {
					parts := strings.Split(host, "@")
					if len(parts) == 2 {
						user = parts[0]
						host = parts[1]
					}
				}

				// 检查必需参数
				if user == "" {
					return fmt.Errorf("username is required when host is not in SSH config file. Use -u flag or user@host format")
				}

				// Create SSH config
				sshConfig := &ssh.Config{
					Host:     host,
					Port:     port,
					Username: user,
					KeyPath:  keyPath,
					Password: password,
					Timeout:  time.Duration(timeout) * time.Second,
				}

				client = ssh.NewClientWithLogger(sshConfig, logger)
			}

			// 获取SSH配置信息
			sshConfig := client.GetConfig()
			logger.Infof("Connecting to %s@%s:%s...", sshConfig.Username, sshConfig.Host, sshConfig.Port)
			if err := client.Connect(); err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			logger.Infof("Connected successfully")

			// Create IDE installer with logger
			ideInstaller := ide.NewInstallerWithOptions(client, ide.IDE(ideType), nil, logger)

			// Check if IDE is installed
			logger.Infof("Checking if %s is installed...", ideType)
			installed, err := ideInstaller.IsInstalled()
			if err != nil {
				return fmt.Errorf("failed to check IDE installation: %w", err)
			}

			// Install IDE if not installed
			if !installed {
				logger.Infof("%s is not installed. Installing...", ideType)
				if err := ideInstaller.Install(); err != nil {
					return fmt.Errorf("failed to install IDE: %w", err)
				}
				logger.Infof("%s installed successfully", ideType)
			} else {
				logger.Infof("%s is already installed", ideType)
			}

			// Start IDE
			defaultPort := ideInstaller.GetDefaultPort()
			logger.Infof("Starting %s on port %d...", ideType, defaultPort)
			if err := ideInstaller.Start(defaultPort); err != nil {
				return fmt.Errorf("failed to start IDE: %w", err)
			}
			logger.Infof("%s started on port %d", ideType, defaultPort)

			// Create tunnel manager
			tunnelManager := tunnel.NewTunnelManagerWithLogger(logger)

			// Parse forward ports
			var forwardConfigs []tunnel.ForwardConfig
			if auto {
				forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{AutoDetect: true})
			} else {
				for _, forward := range forwards {
					parts := strings.Split(forward, ":")
					if len(parts) == 1 {
						// Single port: forward remote port to same local port
						port, err := strconv.Atoi(parts[0])
						if err != nil {
							return fmt.Errorf("invalid port: %s", parts[0])
						}
						forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{
							LocalPort:  port,
							RemotePort: port,
						})
					} else if len(parts) == 2 {
						// Local:Remote port mapping
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

				// Always forward IDE port
				forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{
					LocalPort:  defaultPort,
					RemotePort: defaultPort,
				})
			}

			// Create port forwards
			portResults, err := tunnel.CreatePortForwards(client, forwardConfigs, tunnelManager)
			if err != nil {
				return fmt.Errorf("failed to create port forwards: %w", err)
			}

			// List active tunnels
			tunnels := tunnelManager.ListTunnels()
			logger.Infof("Active port forwards:")
			for name, info := range tunnels {
				logger.Infof("  %s: localhost:%d -> remote:%d", name, info.LocalPort, info.RemotePort)
			}

			// 查找IDE端口的实际转发端口
			actualIDEPort := defaultPort
			foundInResults := false

			// 首先从portResults中查找
			for _, result := range portResults {
				if result.RemotePort == defaultPort {
					actualIDEPort = result.ActualPort
					foundInResults = true
					break
				}
			}

			// 如果没有在portResults中找到，从隧道管理器中查找
			if !foundInResults {
				tunnels := tunnelManager.ListTunnels()
				for _, info := range tunnels {
					// 查找转发到IDE远程端口的隧道
					if info.RemotePort == defaultPort {
						actualIDEPort = info.LocalPort
						break
					}
				}
			}

			logger.Infof("%s is now accessible at http://localhost:%d", ideType, actualIDEPort)
			logger.Infof("Press Ctrl+C to stop...")

			// Wait for interrupt
			select {
			case <-cmd.Context().Done():
				logger.Infof("Stopping...")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH username")
	cmd.Flags().StringVarP(&port, "port", "p", "22", "SSH port")
	cmd.Flags().StringVar(&keyPath, "key", "", "SSH private key path")
	cmd.Flags().StringVar(&password, "password", "", "SSH password")
	cmd.Flags().StringVar(&ideType, "ide", "vscode", "Web IDE type (vscode, code-server)")
	cmd.Flags().StringSliceVar(&forwards, "forward", []string{}, "Ports to forward (e.g., 3000, 8080:80)")
	cmd.Flags().BoolVar(&auto, "auto", false, "Auto-detect and forward web service ports")
	cmd.Flags().IntVar(&timeout, "timeout", 30, "SSH connection timeout in seconds")

	return cmd
}

func newForwardCmd() *cobra.Command {
	var (
		user     string
		port     string
		keyPath  string
		password string
		forwards []string
		auto     bool
		timeout  int
	)

	cmd := &cobra.Command{
		Use:   "forward [host]",
		Short: "Forward ports from remote host to local machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 获取logger
			logger := logging.GetGlobalLogger()
			host := args[0]

			// Parse host if it contains user@host format
			if strings.Contains(host, "@") {
				parts := strings.Split(host, "@")
				if len(parts) == 2 {
					user = parts[0]
					host = parts[1]
				}
			}

			var client *ssh.Client
			var err error

			// 检查是否是SSH配置文件中的主机
			parser := ssh.NewSSHConfigParser()
			_, sshErr := parser.GetHost(host)
			if sshErr == nil {
				// 从SSH配置文件创建客户端，使用命令行参数覆盖
				overrideConfig := &ssh.Config{
					Host: host,

					Username: user,
					KeyPath:  keyPath,
					Password: password,
					Timeout:  time.Duration(timeout) * time.Second,
				}
				// 只有当用户显式提供了-p参数时才覆盖端口
				if port != "22" {
					overrideConfig.Port = port
				}
				client, err = ssh.NewClientFromSSHConfigWithLogger(host, overrideConfig, logger)
				if err != nil {
					return fmt.Errorf("failed to create client from SSH config: %w", err)
				}
			} else {
				// 检查是否是特殊主机模式的错误
				if strings.Contains(sshErr.Error(), "is a special pattern") {
					return fmt.Errorf("cannot connect to %s: %v", host, sshErr)
				}

				// 如果不是SSH配置文件中的主机，使用传统方式
				// Parse host if it contains user@host format
				if strings.Contains(host, "@") {
					parts := strings.Split(host, "@")
					if len(parts) == 2 {
						user = parts[0]
						host = parts[1]
					}
				}

				// 检查必需参数
				if user == "" {
					return fmt.Errorf("username is required when host is not in SSH config file. Use -u flag or user@host format")
				}

				// Create SSH config
				sshConfig := &ssh.Config{
					Host:     host,
					Port:     port,
					Username: user,
					KeyPath:  keyPath,
					Password: password,
					Timeout:  time.Duration(timeout) * time.Second,
				}

				client = ssh.NewClientWithLogger(sshConfig, logger)
			}
			sshConfig := client.GetConfig()
			logger.Infof("Connecting to %s@%s:%s...", sshConfig.Username, sshConfig.Host, sshConfig.Port)
			if err := client.Connect(); err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			logger.Infof("Connected successfully")

			// Create tunnel manager
			tunnelManager := tunnel.NewTunnelManagerWithLogger(logger)

			// Parse forward ports
			var forwardConfigs []tunnel.ForwardConfig
			if auto {
				forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{AutoDetect: true})
			} else {
				for _, forward := range forwards {
					parts := strings.Split(forward, ":")
					if len(parts) == 1 {
						// Single port: forward remote port to same local port
						port, err := strconv.Atoi(parts[0])
						if err != nil {
							return fmt.Errorf("invalid port: %s", parts[0])
						}
						forwardConfigs = append(forwardConfigs, tunnel.ForwardConfig{
							LocalPort:  port,
							RemotePort: port,
						})
					} else if len(parts) == 2 {
						// Local:Remote port mapping
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
			}

			// Create port forwards
			_, err = tunnel.CreatePortForwards(client, forwardConfigs, tunnelManager)
			if err != nil {
				return fmt.Errorf("failed to create port forwards: %w", err)
			}

			// List active tunnels
			tunnels := tunnelManager.ListTunnels()
			logger.Infof("Active port forwards:")
			for name, info := range tunnels {
				logger.Infof("  %s: localhost:%d -> remote:%d", name, info.LocalPort, info.RemotePort)
			}

			logger.Infof("Press Ctrl+C to stop...")

			// Wait for interrupt
			select {
			case <-cmd.Context().Done():
				logger.Infof("Stopping...")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH username")
	cmd.Flags().StringVarP(&port, "port", "p", "22", "SSH port")
	cmd.Flags().StringVar(&keyPath, "key", "", "SSH private key path")
	cmd.Flags().StringVar(&password, "password", "", "SSH password")
	cmd.Flags().StringSliceVar(&forwards, "ports", []string{}, "Ports to forward (e.g., 3000, 8080:80)")
	cmd.Flags().BoolVar(&auto, "auto", false, "Auto-detect and forward web service ports")
	cmd.Flags().IntVar(&timeout, "timeout", 30, "SSH connection timeout in seconds")

	return cmd
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List hosts from SSH config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			// 获取logger
			logger := logging.GetGlobalLogger()

			parser := ssh.NewSSHConfigParser()
			hosts, err := parser.ListHosts()
			if err != nil {
				return fmt.Errorf("failed to list SSH hosts: %w", err)
			}

			if len(hosts) == 0 {
				logger.Infof("No hosts found in SSH config file")
				return nil
			}

			logger.Infof("Hosts from SSH config file:")
			for _, host := range hosts {
				logger.Infof("  %s", host)
			}

			return nil
		},
	}

	return cmd
}
