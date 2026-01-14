package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"devssh/pkg/config"
	"devssh/pkg/ide"
	"devssh/pkg/ssh"
	"devssh/pkg/tunnel"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "devssh",
		Short:   "DevSSH - SSH-based remote development environment setup",
		Version: version,
	}

	rootCmd.AddCommand(
		newConnectCmd(),
		newInstallCmd(),
		newForwardCmd(),
		newListCmd(),
		newStopCmd(),
		newSSHHostsCmd(),
		newImportSSHCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
		install  bool
		forwards []string
		auto     bool
		timeout  int
	)

	cmd := &cobra.Command{
		Use:   "connect [host]",
		Short: "Connect to remote host and setup development environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]

			var client *ssh.Client
			var err error

			// 检查是否是SSH配置文件中的主机
			parser := ssh.NewSSHConfigParser()
			_, sshErr := parser.GetHost(host)
			if sshErr == nil {
				// 从SSH配置文件创建客户端
				client, err = ssh.NewClientFromSSHConfig(host)
				if err != nil {
					return fmt.Errorf("failed to create client from SSH config: %w", err)
				}
			} else {
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

				client = ssh.NewClient(sshConfig)
			}

			// 获取SSH配置信息
			sshConfig := client.GetConfig()
			fmt.Printf("Connecting to %s@%s:%s...\n", sshConfig.Username, sshConfig.Host, sshConfig.Port)
			if err := client.Connect(); err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			fmt.Println("Connected successfully")

			// Create IDE installer
			ideInstaller := ide.NewInstaller(client, ide.IDE(ideType))

			// Install IDE if requested
			if install {
				fmt.Printf("Installing %s...\n", ideType)
				if err := ideInstaller.Install(); err != nil {
					return fmt.Errorf("failed to install IDE: %w", err)
				}
				fmt.Printf("%s installed successfully\n", ideType)
			}

			// Check if IDE is installed
			installed, err := ideInstaller.IsInstalled()
			if err != nil {
				return fmt.Errorf("failed to check IDE installation: %w", err)
			}

			if !installed {
				return fmt.Errorf("%s is not installed. Use --install flag to install it", ideType)
			}

			// Start IDE
			defaultPort := ideInstaller.GetDefaultPort()
			fmt.Printf("Starting %s on port %d...\n", ideType, defaultPort)
			if err := ideInstaller.Start(defaultPort); err != nil {
				return fmt.Errorf("failed to start IDE: %w", err)
			}
			fmt.Printf("%s started on port %d\n", ideType, defaultPort)

			// Create tunnel manager
			tunnelManager := tunnel.NewTunnelManager()

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
			if err := tunnel.CreatePortForwards(client, forwardConfigs, tunnelManager); err != nil {
				return fmt.Errorf("failed to create port forwards: %w", err)
			}

			// List active tunnels
			tunnels := tunnelManager.ListTunnels()
			fmt.Println("\nActive port forwards:")
			for name, info := range tunnels {
				fmt.Printf("  %s: localhost:%d -> remote:%d\n", name, info.LocalPort, info.RemotePort)
			}

			fmt.Printf("\n%s is now accessible at http://localhost:%d\n", ideType, defaultPort)
			fmt.Println("\nPress Ctrl+C to stop...")

			// Wait for interrupt
			select {
			case <-cmd.Context().Done():
				fmt.Println("\nStopping...")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH username")
	cmd.Flags().StringVarP(&port, "port", "p", "22", "SSH port")
	cmd.Flags().StringVar(&keyPath, "key", "", "SSH private key path")
	cmd.Flags().StringVar(&password, "password", "", "SSH password")
	cmd.Flags().StringVar(&ideType, "ide", "vscode", "Web IDE type (vscode, code-server)")
	cmd.Flags().BoolVar(&install, "install", true, "Install IDE if not present")
	cmd.Flags().StringSliceVar(&forwards, "forward", []string{}, "Ports to forward (e.g., 3000, 8080:80)")
	cmd.Flags().BoolVar(&auto, "auto", false, "Auto-detect and forward web service ports")
	cmd.Flags().IntVar(&timeout, "timeout", 30, "SSH connection timeout in seconds")

	return cmd
}

func newInstallCmd() *cobra.Command {
	var (
		user     string
		port     string
		keyPath  string
		password string
		ideType  string
		timeout  int
	)

	cmd := &cobra.Command{
		Use:   "install [host]",
		Short: "Install Web IDE on remote host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			if sshHosts, _ := parser.ListHosts(); len(sshHosts) > 0 {
				for _, sshHost := range sshHosts {
					if sshHost == host {
						// 从SSH配置文件创建客户端
						client, err = ssh.NewClientFromSSHConfig(host)
						if err != nil {
							return fmt.Errorf("failed to create client from SSH config: %w", err)
						}
						break
					}
				}
			}

			// 如果不是SSH配置文件中的主机，使用传统方式
			if client == nil {
				// Parse host if it contains user@host format
				if strings.Contains(host, "@") {
					parts := strings.Split(host, "@")
					if len(parts) == 2 {
						user = parts[0]
						host = parts[1]
					}
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

				client = ssh.NewClient(sshConfig)
			}

			// 获取SSH配置信息
			sshConfig := client.GetConfig()
			fmt.Printf("Connecting to %s@%s:%s...\n", sshConfig.Username, sshConfig.Host, sshConfig.Port)
			if err := client.Connect(); err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			fmt.Println("Connected successfully")

			// Create IDE installer
			ideInstaller := ide.NewInstaller(client, ide.IDE(ideType))

			// Check if already installed
			installed, err := ideInstaller.IsInstalled()
			if err != nil {
				return fmt.Errorf("failed to check installation: %w", err)
			}

			if installed {
				fmt.Printf("%s is already installed\n", ideType)
				return nil
			}

			// Install IDE
			fmt.Printf("Installing %s...\n", ideType)
			if err := ideInstaller.Install(); err != nil {
				return fmt.Errorf("failed to install IDE: %w", err)
			}

			fmt.Printf("%s installed successfully\n", ideType)
			return nil
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH username")
	cmd.Flags().StringVarP(&port, "port", "p", "22", "SSH port")
	cmd.Flags().StringVar(&keyPath, "key", "", "SSH private key path")
	cmd.Flags().StringVar(&password, "password", "", "SSH password")
	cmd.Flags().StringVar(&ideType, "ide", "vscode", "Web IDE type (vscode, code-server)")
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
			if sshHosts, _ := parser.ListHosts(); len(sshHosts) > 0 {
				for _, sshHost := range sshHosts {
					if sshHost == host {
						// 从SSH配置文件创建客户端
						client, err = ssh.NewClientFromSSHConfig(host)
						if err != nil {
							return fmt.Errorf("failed to create client from SSH config: %w", err)
						}
						break
					}
				}
			}

			// 如果不是SSH配置文件中的主机，使用传统方式
			if client == nil {
				// Parse host if it contains user@host format
				if strings.Contains(host, "@") {
					parts := strings.Split(host, "@")
					if len(parts) == 2 {
						user = parts[0]
						host = parts[1]
					}
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

				client = ssh.NewClient(sshConfig)
			}

			// 获取SSH配置信息
			sshConfig := client.GetConfig()
			fmt.Printf("Connecting to %s@%s:%s...\n", sshConfig.Username, sshConfig.Host, sshConfig.Port)
			if err := client.Connect(); err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			fmt.Println("Connected successfully")

			// Create tunnel manager
			tunnelManager := tunnel.NewTunnelManager()

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
			if err := tunnel.CreatePortForwards(client, forwardConfigs, tunnelManager); err != nil {
				return fmt.Errorf("failed to create port forwards: %w", err)
			}

			// List active tunnels
			tunnels := tunnelManager.ListTunnels()
			fmt.Println("\nActive port forwards:")
			for name, info := range tunnels {
				fmt.Printf("  %s: localhost:%d -> remote:%d\n", name, info.LocalPort, info.RemotePort)
			}

			fmt.Println("\nPress Ctrl+C to stop...")

			// Wait for interrupt
			select {
			case <-cmd.Context().Done():
				fmt.Println("\nStopping...")
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
		Short: "List active connections",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if len(cfg.Connections) == 0 {
				fmt.Println("No active connections")
				return nil
			}

			fmt.Println("Active connections:")
			for id, conn := range cfg.Connections {
				fmt.Printf("  %s: %s@%s:%s (IDE: %s, Port: %d)\n",
					id, conn.Username, conn.Host, conn.Port, conn.IDE, conn.LocalPort)
			}

			return nil
		},
	}

	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop [connection-id]",
		Short: "Stop a connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			connectionID := args[0]

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if _, exists := cfg.Connections[connectionID]; !exists {
				return fmt.Errorf("connection %s not found", connectionID)
			}

			delete(cfg.Connections, connectionID)

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Connection %s stopped\n", connectionID)
			return nil
		},
	}

	return cmd
}

func newSSHHostsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-hosts",
		Short: "List hosts from SSH config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			parser := ssh.NewSSHConfigParser()
			hosts, err := parser.ListHosts()
			if err != nil {
				return fmt.Errorf("failed to list SSH hosts: %w", err)
			}

			if len(hosts) == 0 {
				fmt.Println("No hosts found in SSH config file")
				return nil
			}

			fmt.Println("Hosts from SSH config file:")
			for _, host := range hosts {
				fmt.Printf("  %s\n", host)
			}

			return nil
		},
	}

	return cmd
}

func newImportSSHCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-ssh",
		Short: "Import hosts from SSH config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			imported, err := cfg.ImportSSHHosts()
			if err != nil {
				return fmt.Errorf("failed to import SSH hosts: %w", err)
			}

			if imported == 0 {
				fmt.Println("No new hosts imported from SSH config file")
			} else {
				fmt.Printf("Successfully imported %d hosts from SSH config file\n", imported)
			}

			return nil
		},
	}

	return cmd
}
