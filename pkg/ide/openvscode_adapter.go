package ide

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devssh/pkg/download"
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
		values[openvscode.VersionOption] = config.OptionValue{Value: "v1.105.1"}
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

	// 本地下载文件
	localPath, err := s.downloadLocally(url)
	if err != nil {
		return fmt.Errorf("failed to download locally: %w", err)
	}
	defer os.Remove(localPath)

	// 上传到远程服务器
	remotePath := "~/openvscode-server.tar.gz"
	if err := s.uploadToRemote(localPath, remotePath); err != nil {
		return fmt.Errorf("failed to upload to remote: %w", err)
	}

	// 在远程服务器解压安装
	if err := s.extractOnRemote(remotePath); err != nil {
		return fmt.Errorf("failed to extract on remote: %w", err)
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

// downloadLocally 本地下载文件
func (s *SSHOpenVSCodeServer) downloadLocally(url string) (string, error) {
	cacheDir, err := s.getCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to get cache directory: %w", err)
	}

	downloader := download.NewLocalDownloader(cacheDir, s.logger)
	return downloader.Download(url)
}

// uploadToRemote 上传文件到远程服务器
func (s *SSHOpenVSCodeServer) uploadToRemote(localPath, remotePath string) error {
	scpClient := ssh.NewSCPClient(s.sshClient)
	return scpClient.Upload(localPath, remotePath)
}

// extractOnRemote 在远程服务器解压文件
func (s *SSHOpenVSCodeServer) extractOnRemote(remotePath string) error {
	extractScript := `
#!/bin/bash
set -e

# Create Path
mkdir -p ~/.openvscode-server

# Extract File
echo "Extracting openvscode-server..."
tar -xzf ~/openvscode-server.tar.gz -C ~/.openvscode-server --strip-components=1

# Clean temp file
rm -f ~/openvscode-server.tar.gz

if [ $? -eq 0 ]; then
	echo "openvscode-server extracted successfully"
else
	echo "Failed to extract openvscode-server"
	exit 1
fi
`

	_, err := s.sshClient.RunCommand(extractScript)
	return err
}

// getCacheDir 获取缓存目录
func (s *SSHOpenVSCodeServer) getCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "devssh", "openvscode")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir, nil
}

// IsProcessRunning 检查openvscode进程是否在运行
func (s *SSHOpenVSCodeServer) IsProcessRunning(port int) (bool, error) {
	if !s.sshClient.IsConnected() {
		return false, fmt.Errorf("SSH client not connected")
	}

	// 方法1：检查进程命令行（最精确）- 改进版本
	cmd1 := fmt.Sprintf("ps aux | grep -E 'openvscode.*--port[[:space:]]+%d|openvscode.*--port=%d' | grep -v grep", port, port)
	output1, err1 := s.sshClient.RunCommand(cmd1)
	if err1 == nil && strings.TrimSpace(output1) != "" {
		s.logger.Debugf("Found openvscode process via ps command")
		return true, nil
	}

	// 方法2：检查端口占用进程 - 改进版本
	cmd2 := fmt.Sprintf("lsof -i :%d 2>/dev/null | grep -i openvscode", port)
	output2, err2 := s.sshClient.RunCommand(cmd2)
	if err2 == nil && strings.TrimSpace(output2) != "" {
		s.logger.Debugf("Found openvscode process via lsof command")
		return true, nil
	}

	// 方法3：检查进程监听端口 - 改进版本
	cmd3 := fmt.Sprintf("ss -tulpn 2>/dev/null | grep ':%d' | grep -i openvscode", port)
	output3, err3 := s.sshClient.RunCommand(cmd3)
	if err3 == nil && strings.TrimSpace(output3) != "" {
		s.logger.Debugf("Found openvscode process via ss command")
		return true, nil
	}

	// 方法4：检查进程PID文件
	cmd4 := fmt.Sprintf("test -f /tmp/openvscode-server-%d.pid && ps -p $(cat /tmp/openvscode-server-%d.pid) >/dev/null 2>&1 && echo running", port, port)
	output4, err4 := s.sshClient.RunCommand(cmd4)
	if err4 == nil && strings.Contains(output4, "running") {
		s.logger.Debugf("Found openvscode process via PID file")
		return true, nil
	}

	// 方法5：检查网络连接
	cmd5 := fmt.Sprintf("timeout 1 bash -c 'echo > /dev/tcp/localhost/%d' 2>/dev/null && echo port_open", port)
	output5, err5 := s.sshClient.RunCommand(cmd5)
	if err5 == nil && strings.Contains(output5, "port_open") {
		// 端口开放，检查是否是openvscode
		cmd5b := fmt.Sprintf("curl -s http://localhost:%d | grep -i 'vscode\\|openvscode' 2>/dev/null || true", port)
		output5b, _ := s.sshClient.RunCommand(cmd5b)
		if strings.Contains(strings.ToLower(output5b), "vscode") || strings.Contains(strings.ToLower(output5b), "openvscode") {
			s.logger.Debugf("Found openvscode process via HTTP check")
			return true, nil
		}
	}

	// 所有检测方法都失败，认为进程不存在
	return false, nil
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

	// 严格检查进程是否已在运行
	running, err := s.IsProcessRunning(port)
	if err != nil {
		s.logger.Warnf("Failed to check if process is running: %v", err)
	} else if running {
		s.logger.Infof("openvscode-server is already running on port %d, skipping startup", port)
		return nil
	}

	// 清理可能存在的旧PID文件
	cleanupCmd := fmt.Sprintf("rm -f /tmp/openvscode-server-%d.pid", port)
	s.sshClient.RunCommand(cleanupCmd)

	s.logger.Infof("Starting openvscode-server on port %d...", port)

	// 启动命令，创建PID文件
	startScript := fmt.Sprintf(`
#!/bin/bash
set -e

PORT=%d
PID_FILE="/tmp/openvscode-server-${PORT}.pid"
LOG_FILE="/tmp/openvscode-${PORT}.log"

# 再次检查端口是否被占用
if lsof -i :${PORT} >/dev/null 2>&1; then
    echo "Port ${PORT} is already in use"
    exit 1
fi

# 启动openvscode-server
~/.openvscode-server/bin/openvscode-server \
    --host 0.0.0.0 \
    --port ${PORT} \
    --without-connection-token \
    > "${LOG_FILE}" 2>&1 &

SERVER_PID=$!

# 保存PID
echo ${SERVER_PID} > "${PID_FILE}"

# 等待进程启动
for i in {1..30}; do
    if ps -p ${SERVER_PID} >/dev/null 2>&1; then
        # 检查端口是否开始监听
        if timeout 1 bash -c "echo > /dev/tcp/localhost/${PORT}" 2>/dev/null; then
            echo "openvscode-server started successfully on port ${PORT} (PID: ${SERVER_PID})"
            exit 0
        fi
    else
        echo "Process ${SERVER_PID} died unexpectedly"
        rm -f "${PID_FILE}"
        exit 1
    fi
    sleep 1
done

echo "Timeout waiting for openvscode-server to start"
kill ${SERVER_PID} 2>/dev/null || true
rm -f "${PID_FILE}"
exit 1
`, port)

	output, err := s.sshClient.RunCommand(startScript)
	if err != nil {
		return fmt.Errorf("failed to start openvscode-server: %w, output: %s", err, output)
	}

	// 验证进程确实在运行
	time.Sleep(2 * time.Second)
	verifyRunning, verifyErr := s.IsProcessRunning(port)
	if verifyErr != nil {
		s.logger.Warnf("Failed to verify process startup: %v", verifyErr)
	} else if !verifyRunning {
		return fmt.Errorf("openvscode-server failed to start on port %d", port)
	}

	s.logger.Infof("openvscode-server started successfully on port %d", port)
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
		version = "v1.105.1" // 默认版本
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
