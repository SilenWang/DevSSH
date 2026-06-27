package main

import (
	"fmt"

	"devssh/pkg/agent"
	"devssh/pkg/logging"

	"github.com/spf13/cobra"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage VSCodium installation and running",
		Long: `Manage VSCodium (VSCodium reh-web) installation and running on the current machine.

Commands:
  install     Download and install VSCodium reh-web
  start       Start VSCodium
  stop        Stop VSCodium
  uninstall   Uninstall VSCodium and clean up
  is-running  Check if VSCodium is running
  get-port    Get the port VSCodium is running on

Examples:
  devssh agent install
  devssh agent install --version 1.116.02821
  devssh agent install --local-tar /path/to/vscodium-reh-web.tar.gz
  devssh agent start --port 10081
  devssh agent stop
  devssh agent uninstall
  devssh agent is-running
  devssh agent get-port
`,
	}

	cmd.AddCommand(
		newAgentInstallCmd(),
		newAgentStartCmd(),
		newAgentStopCmd(),
		newAgentUninstallCmd(),
		newAgentIsRunningCmd(),
		newAgentGetPortCmd(),
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
		Short: "Download and install VSCodium reh-web",
		Long: `Download and install VSCodium reh-web to the working directory.
If already installed, this command will be skipped.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if runner.IsInstalled() {
				logging.Infof("VSCodium is already installed")
				return nil
			}

			logging.Infof("Installing VSCodium...")

			if localTar != "" {
				if err := runner.InstallFromTar(localTar, version); err != nil {
					return fmt.Errorf("failed to install VSCodium from local tar: %w", err)
				}
			} else {
				if err := runner.Install(version); err != nil {
					return fmt.Errorf("failed to install VSCodium: %w", err)
				}
			}

			logging.Infof("VSCodium installed successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&version, "version", "1.116.02821", "VSCodium version to install")
	cmd.Flags().StringVar(&localTar, "local-tar", "", "Path to local tar.gz file (use this instead of downloading)")

	return cmd
}

func newAgentStartCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start VSCodium",
		Long: `Start VSCodium reh-web on the specified port.
If VSCodium is already running, this command will be skipped.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if !runner.IsInstalled() {
				return fmt.Errorf("VSCodium is not installed. Run 'devssh agent install' first")
			}

			if runner.IsRunning() {
				logging.Infof("VSCodium is already running")
				return nil
			}

			logging.Infof("Starting VSCodium on port %d...", port)

			if err := runner.Start(port); err != nil {
				return fmt.Errorf("failed to start VSCodium: %w", err)
			}

			logging.Infof("VSCodium started successfully")
			logging.Infof("VSCodium is accessible at http://localhost:%d", port)
			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 10081, "Port to start VSCodium on")

	return cmd
}

func newAgentStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop VSCodium",
		Long:  `Stop the running VSCodium instance.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if !runner.IsRunning() {
				logging.Infof("VSCodium is not running")
				return nil
			}

			logging.Infof("Stopping VSCodium...")

			if err := runner.Stop(); err != nil {
				return fmt.Errorf("failed to stop VSCodium: %w", err)
			}

			logging.Infof("VSCodium stopped successfully")
			return nil
		},
	}

	return cmd
}

func newAgentUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall VSCodium and clean up",
		Long:  `Stop VSCodium and remove all installed files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if !runner.IsInstalled() && !runner.IsRunning() {
				logging.Infof("VSCodium is not installed")
				return nil
			}

			logging.Infof("Uninstalling VSCodium...")

			if err := runner.Uninstall(); err != nil {
				return fmt.Errorf("failed to uninstall VSCodium: %w", err)
			}

			return nil
		},
	}

	return cmd
}

func newAgentIsRunningCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "is-running",
		Short: "Check if VSCodium is running",
		Long:  `Check if VSCodium (VSCodium reh-web) is currently running.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if runner.IsRunning() {
				fmt.Println("running")
			} else {
				fmt.Println("not_running")
			}
			return nil
		},
	}

	return cmd
}

func newAgentGetPortCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-port",
		Short: "Get the port VSCodium is running on",
		Long:  `Get the port number that VSCodium (VSCodium reh-web) is running on.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := agent.NewRunner()
			if err != nil {
				return fmt.Errorf("failed to create runner: %w", err)
			}

			if !runner.IsRunning() {
				return nil
			}

			port := runner.GetRunningPort()
			fmt.Printf("%d\n", port)
			return nil
		},
	}

	return cmd
}
