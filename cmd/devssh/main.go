package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "devssh",
		Short:   "DevSSH - SSH-based remote development environment",
		Long:    "DevSSH is a tool that sets up remote development environments via SSH, installs web IDEs, and forwards ports to local machine.",
		Version: version,
	}

	// 添加子命令
	rootCmd.AddCommand(newConnectCmd())
	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newForwardCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newStopCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect [host]",
		Short: "Connect to remote host and setup development environment",
		Long:  "Establish SSH connection to remote host, install web IDE if needed, and forward ports.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			fmt.Printf("Connecting to %s...\n", host)

			// TODO: 实现连接逻辑
			return nil
		},
	}

	cmd.Flags().StringP("user", "u", "", "SSH username")
	cmd.Flags().StringP("port", "p", "22", "SSH port")
	cmd.Flags().String("key", "", "SSH private key path")
	cmd.Flags().String("ide", "vscode", "Web IDE to install (vscode, code-server, etc.)")
	cmd.Flags().Bool("install", true, "Install web IDE if not present")
	cmd.Flags().StringSlice("forward", []string{}, "Ports to forward (e.g., 3000, 8080:80)")

	return cmd
}

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [host]",
		Short: "Install web IDE on remote host",
		Long:  "Install specified web IDE on remote host via SSH.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			fmt.Printf("Installing web IDE on %s...\n", host)

			// TODO: 实现安装逻辑
			return nil
		},
	}

	cmd.Flags().StringP("user", "u", "", "SSH username")
	cmd.Flags().StringP("port", "p", "22", "SSH port")
	cmd.Flags().String("key", "", "SSH private key path")
	cmd.Flags().String("ide", "vscode", "Web IDE to install")

	return cmd
}

func newForwardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forward [host]",
		Short: "Forward ports from remote host to local",
		Long:  "Forward specified ports from remote host to local machine via SSH tunnel.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			fmt.Printf("Forwarding ports from %s...\n", host)

			// TODO: 实现端口转发逻辑
			return nil
		},
	}

	cmd.Flags().StringP("user", "u", "", "SSH username")
	cmd.Flags().StringP("port", "p", "22", "SSH port")
	cmd.Flags().String("key", "", "SSH private key path")
	cmd.Flags().StringSlice("ports", []string{}, "Ports to forward (e.g., 3000, 8080:80)")
	cmd.Flags().Bool("auto", false, "Auto-detect running services and forward their ports")

	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active connections",
		Long:  "List all active DevSSH connections and their status.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Active connections:")
			// TODO: 实现列表逻辑
			return nil
		},
	}
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop [connection-id]",
		Short: "Stop a connection",
		Long:  "Stop an active DevSSH connection.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			connID := args[0]
			fmt.Printf("Stopping connection %s...\n", connID)

			// TODO: 实现停止逻辑
			return nil
		},
	}
}
