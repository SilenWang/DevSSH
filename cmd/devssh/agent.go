package main

import (
	"fmt"

	"devssh/pkg/agent"

	"github.com/spf13/cobra"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage VSCode installation and running",
		Long: `Manage VSCode (openvscode-server) installation and running on the current machine.

Commands:
  install    Download and install openvscode-server
  start      Start VSCode
  stop       Stop VSCode
  uninstall  Uninstall VSCode and clean up

Examples:
  devssh agent install
  devssh agent install --version v1.105.1
  devssh agent install --local-tar /path/to/openvscode.tar.gz
  devssh agent start --port 10080
  devssh agent stop
  devssh agent uninstall
`,
	}

	cmd.AddCommand(
		newAgentInstallCmd(),
		newAgentStartCmd(),
		newAgentStopCmd(),
		newAgentUninstallCmd(),
	)

	return cmd
}

func newAgentInstallCmd() *cobra.Command {
	var (
		version  string
		localTar string
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Download and install openvscode-server",
		Long: `Download and install openvscode-server to the working directory.
If already installed, this command will be skipped.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if runner.IsInstalled() {
				fmt.Println("VSCode is already installed")
				return nil
			}

			fmt.Println("Installing VSCode...")

			if localTar != "" {
				if err := runner.InstallFromTar(localTar); err != nil {
					return fmt.Errorf("failed to install VSCode from local tar: %w", err)
				}
			} else {
				if err := runner.Install(version); err != nil {
					return fmt.Errorf("failed to install VSCode: %w", err)
				}
			}

			fmt.Println("VSCode installed successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&version, "version", "v1.105.1", "VSCode version to install")
	cmd.Flags().StringVar(&localTar, "local-tar", "", "Path to local tar.gz file (use this instead of downloading)")

	return cmd
}

func newAgentStartCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start VSCode",
		Long: `Start openvscode-server on the specified port.
If VSCode is already running, this command will be skipped.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if !runner.IsInstalled() {
				return fmt.Errorf("VSCode is not installed. Run 'devssh agent install' first")
			}

			if runner.IsRunning() {
				fmt.Println("VSCode is already running")
				return nil
			}

			fmt.Printf("Starting VSCode on port %d...\n", port)

			if err := runner.Start(port); err != nil {
				return fmt.Errorf("failed to start VSCode: %w", err)
			}

			fmt.Printf("VSCode started successfully\n")
			fmt.Printf("VSCode is accessible at http://localhost:%d\n", port)
			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 10080, "Port to start VSCode on")

	return cmd
}

func newAgentStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop VSCode",
		Long:  `Stop the running VSCode instance.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if !runner.IsRunning() {
				fmt.Println("VSCode is not running")
				return nil
			}

			fmt.Println("Stopping VSCode...")

			if err := runner.Stop(); err != nil {
				return fmt.Errorf("failed to stop VSCode: %w", err)
			}

			fmt.Println("VSCode stopped successfully")
			return nil
		},
	}

	return cmd
}

func newAgentUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall VSCode and clean up",
		Long:  `Stop VSCode and remove all installed files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if !runner.IsInstalled() && !runner.IsRunning() {
				fmt.Println("VSCode is not installed")
				return nil
			}

			fmt.Println("Uninstalling VSCode...")

			if err := runner.Uninstall(); err != nil {
				return fmt.Errorf("failed to uninstall VSCode: %w", err)
			}

			return nil
		},
	}

	return cmd
}
