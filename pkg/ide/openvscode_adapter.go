package ide

import (
	"fmt"
	"strings"

	"devssh/pkg/ssh"

	"github.com/loft-sh/devpod/pkg/config"
	"github.com/loft-sh/devpod/pkg/ide/openvscode"
	"github.com/loft-sh/log"
)

// SSHOpenVSCodeServer SSH适配器，复用DevPod核心逻辑
type SSHOpenVSCodeServer struct {
	sshClient  *ssh.Client
	logger     log.Logger
	values     map[string]config.OptionValue
	extensions []string
	settings   string
}

// OpenVSCodeOptions 复用DevPod的选项定义
var OpenVSCodeOptions = openvscode.Options

// NewSSHOpenVSCodeServer 创建SSH适配器
func NewSSHOpenVSCodeServer(sshClient *ssh.Client, values map[string]config.OptionValue, logger log.Logger) *SSHOpenVSCodeServer {
	// 设置默认值
	if values == nil {
		values = make(map[string]config.OptionValue)
	}

	// 确保必要的选项有默认值
	if _, ok := values[openvscode.VersionOption]; !ok {
		values[openvscode.VersionOption] = config.OptionValue{Value: "v1.84.2"}
	}
	if _, ok := values[openvscode.BindAddressOption]; !ok {
		values[openvscode.BindAddressOption] = config.OptionValue{Value: ""}
	}
	if _, ok := values[openvscode.OpenOption]; !ok {
		values[openvscode.OpenOption] = config.OptionValue{Value: "false"}
	}
	if _, ok := values[openvscode.ForwardPortsOption]; !ok {
		values[openvscode.ForwardPortsOption] = config.OptionValue{Value: "true"}
	}

	return &SSHOpenVSCodeServer{
		sshClient: sshClient,
		logger:    logger,
		values:    values,
	}
}

// SetExtensions 设置要安装的扩展
func (s *SSHOpenVSCodeServer) SetExtensions(extensions []string) {
	s.extensions = extensions
}

// SetSettings 设置VSCode配置
func (s *SSHOpenVSCodeServer) SetSettings(settings string) {
	s.settings = settings
}

// Install 安装openvscode-server
func (s *SSHOpenVSCodeServer) Install() error {
	if !s.sshClient.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	// 检查是否已经安装
	installed, err := s.IsInstalled()
	if err != nil {
		return fmt.Errorf("failed to check if openvscode is installed: %w", err)
	}

	if installed {
		s.logger.Infof("openvscode-server is already installed")
		return nil
	}

	s.logger.Infof("Installing openvscode-server...")

	// 获取下载URL
	url, err := s.getReleaseUrl()
	if err != nil {
		return fmt.Errorf("failed to get release URL: %w", err)
	}

	// 通过SSH执行安装
	installScript := fmt.Sprintf(`
#!/bin/bash
set -e

echo "Downloading openvscode-server from %s..."

# 创建目录
mkdir -p ~/.openvscode-server

# 下载并解压
if command -v curl &> /dev/null; then
	curl -L "%s" | tar -xz -C ~/.openvscode-server --strip-components=1
elif command -v wget &> /dev/null; then
	wget -qO- "%s" | tar -xz -C ~/.openvscode-server --strip-components=1
else
	echo "Error: curl or wget is required"
	exit 1
fi

if [ $? -eq 0 ]; then
	echo "openvscode-server installed successfully at ~/.openvscode-server"
else
	echo "Failed to install openvscode-server"
	exit 1
fi
`, url, url, url)

	_, err = s.sshClient.RunCommand(installScript)
	if err != nil {
		return fmt.Errorf("failed to install openvscode-server: %w", err)
	}

	s.logger.Infof("openvscode-server installed successfully")

	// 安装扩展
	if len(s.extensions) > 0 {
		s.logger.Infof("Installing extensions...")
		if err := s.InstallExtensions(); err != nil {
			s.logger.Warnf("Failed to install some extensions: %v", err)
		}
	}

	// 安装设置
	if s.settings != "" {
		s.logger.Infof("Installing settings...")
		if err := s.InstallSettings(); err != nil {
			s.logger.Warnf("Failed to install settings: %v", err)
		}
	}

	return nil
}

// InstallExtensions 安装VSCode扩展
func (s *SSHOpenVSCodeServer) InstallExtensions() error {
	if len(s.extensions) == 0 {
		return nil
	}

	for _, extension := range s.extensions {
		s.logger.Infof("Installing extension: %s", extension)
		cmd := fmt.Sprintf("~/.openvscode-server/bin/openvscode-server --install-extension '%s'", extension)
		output, err := s.sshClient.RunCommand(cmd)
		if err != nil {
			s.logger.Warnf("Failed to install extension %s: %v", extension, err)
			s.logger.Debugf("Output: %s", output)
		} else {
			s.logger.Infof("Successfully installed extension: %s", extension)
		}
	}

	return nil
}

// InstallSettings 安装VSCode设置
func (s *SSHOpenVSCodeServer) InstallSettings() error {
	if s.settings == "" {
		return nil
	}

	// 创建设置目录
	mkdirCmd := "mkdir -p ~/.openvscode-server/data/Machine"
	_, err := s.sshClient.RunCommand(mkdirCmd)
	if err != nil {
		return fmt.Errorf("failed to create settings directory: %w", err)
	}

	writeCmd := fmt.Sprintf("cat > ~/.openvscode-server/data/Machine/settings.json << 'EOF'\n%s\nEOF", s.settings)
	_, err = s.sshClient.RunCommand(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	s.logger.Infof("Settings installed successfully")
	return nil
}

// Start 启动openvscode-server
func (s *SSHOpenVSCodeServer) Start(port int) error {
	if !s.sshClient.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	// 检查是否已安装
	installed, err := s.IsInstalled()
	if err != nil {
		return fmt.Errorf("failed to check installation: %w", err)
	}
	if !installed {
		return fmt.Errorf("openvscode-server is not installed")
	}

	s.logger.Infof("Starting openvscode-server on port %d...", port)

	// 启动命令
	cmd := fmt.Sprintf("nohup ~/.openvscode-server/bin/openvscode-server --host 0.0.0.0 --port %d --without-connection-token > /tmp/openvscode.log 2>&1 &", port)

	output, err := s.sshClient.RunCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to start openvscode-server: %w, output: %s", err, output)
	}

	s.logger.Infof("openvscode-server started successfully")
	return nil
}

// IsInstalled 检查是否已安装
func (s *SSHOpenVSCodeServer) IsInstalled() (bool, error) {
	if !s.sshClient.IsConnected() {
		return false, fmt.Errorf("SSH client not connected")
	}

	checkCmd := "test -f ~/.openvscode-server/bin/openvscode-server && echo installed"
	output, err := s.sshClient.RunCommand(checkCmd)
	if err != nil {
		return false, nil
	}

	return strings.Contains(output, "installed"), nil
}

// GetDefaultPort 获取默认端口
func (s *SSHOpenVSCodeServer) GetDefaultPort() int {
	return openvscode.DefaultVSCodePort
}

// getReleaseUrl 获取下载URL（复用DevPod逻辑）
func (s *SSHOpenVSCodeServer) getReleaseUrl() (string, error) {
	// 检测远程系统架构
	arch, err := s.detectArchitecture()
	if err != nil {
		return "", fmt.Errorf("failed to detect architecture: %w", err)
	}

	// 获取版本
	version := OpenVSCodeOptions.GetValue(s.values, openvscode.VersionOption)
	if version == "" {
		version = "v1.84.2" // 默认版本
	}

	// 根据架构生成URL（复用DevPod的模板）
	if arch == "arm64" {
		url := OpenVSCodeOptions.GetValue(s.values, openvscode.DownloadArm64Option)
		if url == "" {
			url = fmt.Sprintf(openvscode.DownloadArm64Template, version, version)
		}
		return url, nil
	} else {
		// 默认为amd64
		url := OpenVSCodeOptions.GetValue(s.values, openvscode.DownloadAmd64Option)
		if url == "" {
			url = fmt.Sprintf(openvscode.DownloadAmd64Template, version, version)
		}
		return url, nil
	}
}

// detectArchitecture 检测远程系统架构
func (s *SSHOpenVSCodeServer) detectArchitecture() (string, error) {
	cmd := "uname -m"
	output, err := s.sshClient.RunCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to detect architecture: %w", err)
	}

	arch := strings.TrimSpace(output)
	switch arch {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "amd64", nil // 默认为amd64
	}
}
