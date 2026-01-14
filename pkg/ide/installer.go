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

type Installer struct {
	sshClient *ssh.Client
	ideType   IDE
	values    map[string]config.OptionValue
	logger    log.Logger
}

func NewInstaller(sshClient *ssh.Client, ideType IDE) *Installer {
	values := map[string]config.OptionValue{
		"FORWARD_PORTS": {Value: "true"},
		"OPEN":          {Value: "false"},
		"BIND_ADDRESS":  {Value: ""},
		"VERSION":       {Value: "v1.84.2"},
	}

	// 创建一个简单的logger
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)

	return &Installer{
		sshClient: sshClient,
		ideType:   ideType,
		values:    values,
		logger:    logger,
	}
}

// NewInstallerWithOptions 创建带有配置选项的安装器
func NewInstallerWithOptions(sshClient *ssh.Client, ideType IDE, values map[string]config.OptionValue, logger log.Logger) *Installer {
	if values == nil {
		values = make(map[string]config.OptionValue)
	}
	if logger == nil {
		logger = log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)
	}

	return &Installer{
		sshClient: sshClient,
		ideType:   ideType,
		values:    values,
		logger:    logger,
	}
}

func (i *Installer) Install() error {
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

func (i *Installer) installOpenVSCode() error {
	// 使用新的SSHOpenVSCodeServer适配器
	server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
	return server.Install()
}

func (i *Installer) Start(port int) error {
	switch i.ideType {
	case VSCode, CodeServer:
		return i.startOpenVSCode(port)
	default:
		return fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *Installer) startOpenVSCode(port int) error {
	// 使用新的SSHOpenVSCodeServer适配器
	server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
	return server.Start(port)
}

func (i *Installer) IsInstalled() (bool, error) {
	switch i.ideType {
	case VSCode, CodeServer:
		// 使用新的SSHOpenVSCodeServer适配器检查
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		return server.IsInstalled()
	default:
		return false, fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *Installer) GetDefaultPort() int {
	switch i.ideType {
	case VSCode, CodeServer:
		// 使用新的SSHOpenVSCodeServer适配器获取默认端口
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		return server.GetDefaultPort()
	default:
		return 8080
	}
}

func (i *Installer) GetName() string {
	return string(i.ideType)
}

func (i *Installer) SetLogger(logger log.Logger) {
	i.logger = logger
}

// SetOpenVSCodeExtensions 设置openvscode扩展
func (i *Installer) SetOpenVSCodeExtensions(extensions []string) {
	if i.ideType == VSCode || i.ideType == CodeServer {
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		server.SetExtensions(extensions)
	}
}

// SetOpenVSCodeSettings 设置openvscode配置
func (i *Installer) SetOpenVSCodeSettings(settings string) {
	if i.ideType == VSCode || i.ideType == CodeServer {
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		server.SetSettings(settings)
	}
}
