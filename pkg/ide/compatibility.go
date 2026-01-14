package ide

import (
	"os"

	"devssh/pkg/ssh"

	"github.com/loft-sh/devpod/pkg/config"
	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
)

// CompatibilityInstaller 兼容性安装器，自动选择Agent或传统方式
type CompatibilityInstaller struct {
	installer Installer
	useAgent  bool
}

// NewCompatibilityInstaller 创建兼容性安装器
func NewCompatibilityInstaller(sshClient *ssh.Client, ideType IDE) (Installer, error) {
	// 首先尝试使用Agent安装器
	agentInstaller, err := NewAgentInstaller(sshClient, ideType)
	if err == nil {
		return &CompatibilityInstaller{
			installer: agentInstaller,
			useAgent:  true,
		}, nil
	}

	// Agent失败，回退到传统方式
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)
	logger.Warnf("Failed to use agent, falling back to legacy installer: %v", err)

	legacyInstaller := NewInstaller(sshClient, ideType)

	return &CompatibilityInstaller{
		installer: legacyInstaller,
		useAgent:  false,
	}, nil
}

// NewCompatibilityInstallerWithOptions 创建带有选项的兼容性安装器
func NewCompatibilityInstallerWithOptions(sshClient *ssh.Client, ideType IDE, values map[string]config.OptionValue, logger log.Logger) (Installer, error) {
	if logger == nil {
		logger = log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)
	}

	// 首先尝试使用Agent安装器
	agentInstaller, err := NewAgentInstallerWithOptions(sshClient, ideType, values, logger)
	if err == nil {
		return &CompatibilityInstaller{
			installer: agentInstaller,
			useAgent:  true,
		}, nil
	}

	// Agent失败，回退到传统方式
	logger.Warnf("Failed to use agent, falling back to legacy installer: %v", err)

	legacyInstaller := NewInstallerWithOptions(sshClient, ideType, values, logger)

	return &CompatibilityInstaller{
		installer: legacyInstaller,
		useAgent:  false,
	}, nil
}

// Install 安装IDE
func (ci *CompatibilityInstaller) Install() error {
	return ci.installer.Install()
}

// Start 启动IDE
func (ci *CompatibilityInstaller) Start(port int) error {
	return ci.installer.Start(port)
}

// IsInstalled 检查是否已安装
func (ci *CompatibilityInstaller) IsInstalled() (bool, error) {
	return ci.installer.IsInstalled()
}

// GetDefaultPort 获取默认端口
func (ci *CompatibilityInstaller) GetDefaultPort() int {
	return ci.installer.GetDefaultPort()
}

// GetName 获取名称
func (ci *CompatibilityInstaller) GetName() string {
	return ci.installer.GetName()
}

// SetLogger 设置日志器
func (ci *CompatibilityInstaller) SetLogger(logger log.Logger) {
	ci.installer.SetLogger(logger)
}

// SetOpenVSCodeExtensions 设置扩展
func (ci *CompatibilityInstaller) SetOpenVSCodeExtensions(extensions []string) {
	ci.installer.SetOpenVSCodeExtensions(extensions)
}

// SetOpenVSCodeSettings 设置配置
func (ci *CompatibilityInstaller) SetOpenVSCodeSettings(settings string) {
	ci.installer.SetOpenVSCodeSettings(settings)
}

// IsUsingAgent 检查是否使用Agent
func (ci *CompatibilityInstaller) IsUsingAgent() bool {
	return ci.useAgent
}

// GetInstaller 获取底层安装器
func (ci *CompatibilityInstaller) GetInstaller() Installer {
	return ci.installer
}

// Factory 工厂函数，返回兼容性安装器
func Factory(sshClient *ssh.Client, ideType IDE) (Installer, error) {
	return NewCompatibilityInstaller(sshClient, ideType)
}

// FactoryWithOptions 带选项的工厂函数
func FactoryWithOptions(sshClient *ssh.Client, ideType IDE, values map[string]config.OptionValue, logger log.Logger) (Installer, error) {
	return NewCompatibilityInstallerWithOptions(sshClient, ideType, values, logger)
}
