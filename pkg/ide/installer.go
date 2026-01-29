package ide

import (
	"devssh/pkg/logging"
	"devssh/pkg/ssh"

	"github.com/loft-sh/devpod/pkg/config"
	"github.com/loft-sh/log"
)

type IDE string

const (
	VSCode     IDE = "vscode"
	CodeServer IDE = "code-server"
)

type Installer interface {
	Install() error
	Start(port int) error
	IsInstalled() (bool, error)
	GetDefaultPort() int
	GetName() string
	SetLogger(logger log.Logger)
	SetOpenVSCodeExtensions(extensions []string)
	SetOpenVSCodeSettings(settings string)
}

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

	logger := logging.InitDefault()

	return &LegacyInstaller{
		sshClient: sshClient,
		ideType:   ideType,
		values:    values,
		logger:    logger,
	}
}

func NewInstallerWithOptions(sshClient *ssh.Client, ideType IDE, values map[string]config.OptionValue, logger log.Logger) Installer {
	if values == nil {
		values = make(map[string]config.OptionValue)
	}
	if logger == nil {
		logger = logging.InitDefault()
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
		return nil
	}

	switch i.ideType {
	case VSCode, CodeServer:
		return i.installOpenVSCode()
	default:
		return nil
	}
}

func (i *LegacyInstaller) installOpenVSCode() error {
	server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
	return server.Install()
}

func (i *LegacyInstaller) Start(port int) error {
	switch i.ideType {
	case VSCode, CodeServer:
		return i.startOpenVSCode(port)
	default:
		return nil
	}
}

func (i *LegacyInstaller) startOpenVSCode(port int) error {
	server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
	return server.Start(port)
}

func (i *LegacyInstaller) IsInstalled() (bool, error) {
	switch i.ideType {
	case VSCode, CodeServer:
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		return server.IsInstalled()
	default:
		return false, nil
	}
}

func (i *LegacyInstaller) GetDefaultPort() int {
	switch i.ideType {
	case VSCode, CodeServer:
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

func (i *LegacyInstaller) SetOpenVSCodeExtensions(extensions []string) {
	if i.ideType == VSCode || i.ideType == CodeServer {
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		server.SetExtensions(extensions)
	}
}

func (i *LegacyInstaller) SetOpenVSCodeSettings(settings string) {
	if i.ideType == VSCode || i.ideType == CodeServer {
		server := NewSSHOpenVSCodeServer(i.sshClient, i.values, i.logger)
		server.SetSettings(settings)
	}
}
