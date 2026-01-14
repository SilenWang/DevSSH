package ide

import (
	"fmt"
	"os"

	"devssh/pkg/ssh"

	"github.com/loft-sh/devpod/pkg/config"
	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
)

type IDE string

const (
	VSCode     IDE = "vscode"
	CodeServer IDE = "code-server"
)

// Installer IDE安装器接口
type Installer interface {
	// 安装IDE
	Install() error
	// 启动IDE
	Start(port int) error
	// 检查是否已安装
	IsInstalled() (bool, error)
	// 获取默认端口
	GetDefaultPort() int
	// 获取名称
	GetName() string
	// 设置日志器
	SetLogger(logger log.Logger)
	// 设置openvscode扩展
	SetOpenVSCodeExtensions(extensions []string)
	// 设置openvscode配置
	SetOpenVSCodeSettings(settings string)
}

// LegacyInstaller 传统安装器（直接SSH命令执行）
type LegacyInstaller struct {
	sshClient *ssh.Client
	ideType   IDE
	values    map[string]config.OptionValue
	logger    log.Logger
}

func NewInstaller(sshClient *ssh.Client, ideType IDE) Installer {
	values := map[string]config.OptionValue{
		"FORWARD_PORTS": {Value: "true"},
		"OPEN":          {Value: "false"},
		"BIND_ADDRESS":  {Value: ""},
		"VERSION":       {Value: "v1.105.1"},
	}

	// 创建一个简单的logger
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)

	return &LegacyInstaller{
		sshClient: sshClient,
		ideType:   ideType,
		values:    values,
		logger:    logger,
	}
}

// NewInstallerWithOptions 创建带有配置选项的安装器
func NewInstallerWithOptions(sshClient *ssh.Client, ideType IDE, values map[string]config.OptionValue, logger log.Logger) Installer {
	if values == nil {
		values = make(map[string]config.OptionValue)
	}
	if logger == nil {
		logger = log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)
	}

	return &LegacyInstaller{
		sshClient: sshClient,
		ideType:   ideType,
		values:    values,
		logger:    logger,
	}
}

func (i *LegacyInstaller) Install() error {
	if !i.sshClient.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	switch i.ideType {
	case VSCode, CodeServer:
		return i.installOpenVSCode()
	default:
		return fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *LegacyInstaller) installOpenVSCode() error {
	// 使用新的SSHOpenVSCodeServer适配器
	server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
	return server.Install()
}

func (i *LegacyInstaller) Start(port int) error {
	switch i.ideType {
	case VSCode, CodeServer:
		return i.startOpenVSCode(port)
	default:
		return fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *LegacyInstaller) startOpenVSCode(port int) error {
	// 使用新的SSHOpenVSCodeServer适配器
	server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
	return server.Start(port)
}

func (i *LegacyInstaller) IsInstalled() (bool, error) {
	switch i.ideType {
	case VSCode, CodeServer:
		// 使用新的SSHOpenVSCodeServer适配器检查
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		return server.IsInstalled()
	default:
		return false, fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *LegacyInstaller) GetDefaultPort() int {
	switch i.ideType {
	case VSCode, CodeServer:
		// 使用新的SSHOpenVSCodeServer适配器获取默认端口
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		return server.GetDefaultPort()
	default:
		return 8080
	}
}

func (i *LegacyInstaller) GetName() string {
	return string(i.ideType)
}

func (i *LegacyInstaller) SetLogger(logger log.Logger) {
	i.logger = logger
}

// SetOpenVSCodeExtensions 设置openvscode扩展
func (i *LegacyInstaller) SetOpenVSCodeExtensions(extensions []string) {
	if i.ideType == VSCode || i.ideType == CodeServer {
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		server.SetExtensions(extensions)
	}
}

// SetOpenVSCodeSettings 设置openvscode配置
func (i *LegacyInstaller) SetOpenVSCodeSettings(settings string) {
	if i.ideType == VSCode || i.ideType == CodeServer {
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		server.SetSettings(settings)
	}
}
