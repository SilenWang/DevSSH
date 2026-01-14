package ide

import (
	"context"
	"fmt"
	"os"
	"time"

	"devssh/pkg/agent"
	"devssh/pkg/ssh"

	"github.com/loft-sh/devpod/pkg/config"
	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
)

// AgentInstaller 基于Agent的IDE安装器
type AgentInstaller struct {
	agentClient agent.Client
	ideType     IDE
	values      map[string]config.OptionValue
	logger      log.Logger
}

// NewAgentInstaller 创建新的Agent安装器
func NewAgentInstaller(sshClient *ssh.Client, ideType IDE) (*AgentInstaller, error) {
	// 创建默认配置
	sshConfig := sshClient.GetConfig()
	agentConfig := agent.ConfigFromSSH(sshConfig)

	// 创建Agent客户端
	agentClient, err := agent.CreateAgentWithSSH(sshClient, agentConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent client: %w", err)
	}

	// 连接到Agent
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := agentClient.(agent.Client).Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to agent: %w", err)
	}

	// 创建默认值
	values := map[string]config.OptionValue{
		"FORWARD_PORTS": {Value: "true"},
		"OPEN":          {Value: "false"},
		"BIND_ADDRESS":  {Value: ""},
		"VERSION":       {Value: "v1.105.1"},
	}

	// 创建logger
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)

	return &AgentInstaller{
		agentClient: agentClient.(agent.Client),
		ideType:     ideType,
		values:      values,
		logger:      logger,
	}, nil
}

// NewAgentInstallerWithOptions 创建带有配置选项的Agent安装器
func NewAgentInstallerWithOptions(sshClient *ssh.Client, ideType IDE, values map[string]config.OptionValue, logger log.Logger) (*AgentInstaller, error) {
	if values == nil {
		values = make(map[string]config.OptionValue)
	}
	if logger == nil {
		logger = log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)
	}

	// 创建默认配置
	sshConfig := sshClient.GetConfig()
	agentConfig := agent.ConfigFromSSH(sshConfig)

	// 创建Agent客户端
	agentClient, err := agent.CreateAgentWithSSH(sshClient, agentConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent client: %w", err)
	}

	// 连接到Agent
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := agentClient.(agent.Client).Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to agent: %w", err)
	}

	return &AgentInstaller{
		agentClient: agentClient.(agent.Client),
		ideType:     ideType,
		values:      values,
		logger:      logger,
	}, nil
}

// Install 安装IDE
func (ai *AgentInstaller) Install() error {
	// 检查Agent连接
	if !ai.agentClient.IsConnected() {
		return fmt.Errorf("agent client not connected")
	}

	// 检查是否已经安装
	installed, err := ai.IsInstalled()
	if err != nil {
		return fmt.Errorf("failed to check installation: %w", err)
	}

	if installed {
		ai.logger.Infof("%s is already installed", ai.ideType)
		return nil
	}

	ai.logger.Infof("Installing %s via agent...", ai.ideType)

	// 创建IDE配置
	ideConfig := agent.IDEConfig{
		Type:    agent.IDEType(ai.ideType),
		Version: ai.values["VERSION"].Value,
		Port:    ai.GetDefaultPort(),
		Options: ai.convertOptions(),
	}

	// 通过Agent安装IDE
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := ai.agentClient.InstallIDE(ctx, ideConfig); err != nil {
		return fmt.Errorf("failed to install IDE via agent: %w", err)
	}

	ai.logger.Infof("%s installed successfully via agent", ai.ideType)
	return nil
}

// Start 启动IDE
func (ai *AgentInstaller) Start(port int) error {
	// 检查Agent连接
	if !ai.agentClient.IsConnected() {
		return fmt.Errorf("agent client not connected")
	}

	// 检查是否已安装
	installed, err := ai.IsInstalled()
	if err != nil {
		return fmt.Errorf("failed to check installation: %w", err)
	}

	if !installed {
		return fmt.Errorf("%s is not installed", ai.ideType)
	}

	ai.logger.Infof("Starting %s on port %d via agent...", ai.ideType, port)

	// 通过Agent启动IDE
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := ai.agentClient.StartIDE(ctx, agent.IDEType(ai.ideType), port); err != nil {
		return fmt.Errorf("failed to start IDE via agent: %w", err)
	}

	ai.logger.Infof("%s started successfully on port %d", ai.ideType, port)
	return nil
}

// IsInstalled 检查是否已安装
func (ai *AgentInstaller) IsInstalled() (bool, error) {
	// 检查Agent连接
	if !ai.agentClient.IsConnected() {
		return false, fmt.Errorf("agent client not connected")
	}

	// 通过Agent检查IDE状态
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, err := ai.agentClient.GetIDEStatus(ctx, agent.IDEType(ai.ideType))
	if err != nil {
		// 如果获取状态失败，可能IDE未安装
		return false, nil
	}

	// 根据状态判断是否已安装
	// 这里简化处理：如果能够获取到状态，就认为已安装
	return status.Type == agent.IDEType(ai.ideType), nil
}

// GetDefaultPort 获取默认端口
func (ai *AgentInstaller) GetDefaultPort() int {
	switch ai.ideType {
	case VSCode, CodeServer:
		return 8080
	default:
		return 8080
	}
}

// GetName 获取IDE名称
func (ai *AgentInstaller) GetName() string {
	return string(ai.ideType)
}

// SetLogger 设置日志器
func (ai *AgentInstaller) SetLogger(logger log.Logger) {
	ai.logger = logger
}

// SetOpenVSCodeExtensions 设置openvscode扩展
func (ai *AgentInstaller) SetOpenVSCodeExtensions(extensions []string) {
	if ai.ideType == VSCode || ai.ideType == CodeServer {
		// 通过Agent安装扩展
		// 这里需要扩展Agent接口来支持扩展安装
		ai.logger.Warnf("Extension installation via agent not yet implemented")
	}
}

// SetOpenVSCodeSettings 设置openvscode配置
func (ai *AgentInstaller) SetOpenVSCodeSettings(settings string) {
	if ai.ideType == VSCode || ai.ideType == CodeServer {
		// 通过Agent设置配置
		// 这里需要扩展Agent接口来支持配置设置
		ai.logger.Warnf("Settings installation via agent not yet implemented")
	}
}

// GetIDEStatus 获取IDE状态
func (ai *AgentInstaller) GetIDEStatus() (agent.IDEStatus, error) {
	// 检查Agent连接
	if !ai.agentClient.IsConnected() {
		return agent.IDEStatus{}, fmt.Errorf("agent client not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return ai.agentClient.GetIDEStatus(ctx, agent.IDEType(ai.ideType))
}

// ListIDEs 列出所有IDE
func (ai *AgentInstaller) ListIDEs() ([]agent.IDEStatus, error) {
	// 检查Agent连接
	if !ai.agentClient.IsConnected() {
		return nil, fmt.Errorf("agent client not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return ai.agentClient.ListIDEs(ctx)
}

// StopIDE 停止IDE
func (ai *AgentInstaller) StopIDE() error {
	// 检查Agent连接
	if !ai.agentClient.IsConnected() {
		return fmt.Errorf("agent client not connected")
	}

	ai.logger.Infof("Stopping %s via agent...", ai.ideType)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := ai.agentClient.StopIDE(ctx, agent.IDEType(ai.ideType)); err != nil {
		return fmt.Errorf("failed to stop IDE via agent: %w", err)
	}

	ai.logger.Infof("%s stopped successfully", ai.ideType)
	return nil
}

// RestartIDE 重启IDE
func (ai *AgentInstaller) RestartIDE() error {
	// 检查Agent连接
	if !ai.agentClient.IsConnected() {
		return fmt.Errorf("agent client not connected")
	}

	ai.logger.Infof("Restarting %s via agent...", ai.ideType)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := ai.agentClient.RestartIDE(ctx, agent.IDEType(ai.ideType)); err != nil {
		return fmt.Errorf("failed to restart IDE via agent: %w", err)
	}

	ai.logger.Infof("%s restarted successfully", ai.ideType)
	return nil
}

// ExecuteCommand 执行命令
func (ai *AgentInstaller) ExecuteCommand(command string, args ...string) (string, error) {
	// 检查Agent连接
	if !ai.agentClient.IsConnected() {
		return "", fmt.Errorf("agent client not connected")
	}

	req := agent.CommandRequest{
		Command: command,
		Args:    args,
		Timeout: 5 * time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), req.Timeout)
	defer cancel()

	resp, err := ai.agentClient.ExecuteCommand(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to execute command via agent: %w", err)
	}

	if resp.ExitCode != 0 {
		return resp.Stdout, fmt.Errorf("command failed with exit code %d: %s", resp.ExitCode, resp.Stderr)
	}

	return resp.Stdout, nil
}

// Close 关闭安装器
func (ai *AgentInstaller) Close() error {
	if ai.agentClient != nil {
		return ai.agentClient.Close()
	}
	return nil
}

// 私有方法

func (ai *AgentInstaller) convertOptions() map[string]string {
	options := make(map[string]string)
	for key, value := range ai.values {
		options[key] = value.Value
	}
	return options
}

// AgentInstallerFactory Agent安装器工厂
type AgentInstallerFactory struct{}

// NewAgentInstallerFactory 创建Agent安装器工厂
func NewAgentInstallerFactory() *AgentInstallerFactory {
	return &AgentInstallerFactory{}
}

// CreateInstaller 创建安装器
func (f *AgentInstallerFactory) CreateInstaller(sshClient *ssh.Client, ideType IDE) (Installer, error) {
	return NewAgentInstaller(sshClient, ideType)
}

// CreateInstallerWithOptions 创建带有选项的安装器
func (f *AgentInstallerFactory) CreateInstallerWithOptions(sshClient *ssh.Client, ideType IDE, values map[string]config.OptionValue, logger log.Logger) (Installer, error) {
	return NewAgentInstallerWithOptions(sshClient, ideType, values, logger)
}
