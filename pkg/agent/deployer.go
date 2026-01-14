package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devssh/pkg/ssh"
)

// Deployer 负责部署Agent到远程机器
type Deployer struct {
	sshClient *ssh.Client
	binaryMgr *BinaryManager
}

// NewDeployer 创建新的部署器
func NewDeployer(sshClient *ssh.Client) *Deployer {
	return &Deployer{
		sshClient: sshClient,
		binaryMgr: NewBinaryManager(""),
	}
}

// Deploy 部署Agent到远程机器
func (d *Deployer) Deploy(ctx context.Context, config AgentConfig) error {
	// 检查SSH连接
	if !d.sshClient.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	// 检测远程系统
	remoteOS, remoteArch, err := d.detectRemoteSystem(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect remote system: %w", err)
	}

	// 获取Agent二进制文件
	binaryPath, err := d.binaryMgr.GetRemoteBinary(remoteOS, remoteArch, "latest")
	if err != nil {
		return fmt.Errorf("failed to get agent binary: %w", err)
	}

	// 上传二进制文件
	remotePath, err := d.uploadBinary(ctx, binaryPath, config)
	if err != nil {
		return fmt.Errorf("failed to upload binary: %w", err)
	}

	// 创建配置文件
	if err := d.createConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	// 启动Agent
	if err := d.startAgent(ctx, remotePath, config); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	// 等待Agent启动
	if err := d.waitForAgent(ctx, config); err != nil {
		return fmt.Errorf("agent failed to start: %w", err)
	}

	return nil
}

// Undeploy 卸载Agent
func (d *Deployer) Undeploy(ctx context.Context) error {
	// 停止Agent
	if err := d.stopAgent(ctx); err != nil {
		return fmt.Errorf("failed to stop agent: %w", err)
	}

	// 删除Agent文件
	if err := d.cleanupFiles(ctx); err != nil {
		return fmt.Errorf("failed to cleanup files: %w", err)
	}

	return nil
}

// CheckDeployed 检查Agent是否已部署
func (d *Deployer) CheckDeployed(ctx context.Context) (bool, error) {
	// 检查Agent进程
	checkCmd := "ps aux | grep devssh-agent | grep -v grep"
	output, err := d.sshClient.RunCommand(checkCmd)
	if err != nil {
		// 命令执行失败，可能没有grep或ps
		return false, nil
	}

	return strings.Contains(output, "devssh-agent"), nil
}

// GetAgentStatus 获取Agent状态
func (d *Deployer) GetAgentStatus(ctx context.Context) (AgentStatus, error) {
	deployed, err := d.CheckDeployed(ctx)
	if err != nil {
		return StatusError, err
	}

	if !deployed {
		return StatusStopped, nil
	}

	// 尝试连接到Agent获取详细状态
	// 这里简化实现，只检查进程是否存在
	return StatusRunning, nil
}

// UpdateAgent 更新Agent
func (d *Deployer) UpdateAgent(ctx context.Context, version string) error {
	// 停止当前Agent
	if err := d.stopAgent(ctx); err != nil {
		return fmt.Errorf("failed to stop agent: %w", err)
	}

	// 获取配置
	config, err := d.loadConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 重新部署新版本
	return d.Deploy(ctx, config)
}

// 私有方法

func (d *Deployer) detectRemoteSystem(ctx context.Context) (string, string, error) {
	// 检测操作系统
	osCmd := "uname -s"
	osOutput, err := d.sshClient.RunCommand(osCmd)
	if err != nil {
		return "", "", fmt.Errorf("failed to detect OS: %w", err)
	}

	remoteOS := strings.ToLower(strings.TrimSpace(osOutput))

	// 检测架构
	archCmd := "uname -m"
	archOutput, err := d.sshClient.RunCommand(archCmd)
	if err != nil {
		return "", "", fmt.Errorf("failed to detect architecture: %w", err)
	}

	remoteArch := strings.ToLower(strings.TrimSpace(archOutput))

	// 标准化架构名称
	switch remoteArch {
	case "x86_64":
		remoteArch = "amd64"
	case "aarch64":
		remoteArch = "arm64"
	case "armv7l", "armv8l":
		remoteArch = "arm"
	}

	// 标准化操作系统名称
	switch remoteOS {
	case "linux":
		// 已经是标准名称
	case "darwin":
		remoteOS = "darwin"
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", remoteOS)
	}

	return remoteOS, remoteArch, nil
}

func (d *Deployer) uploadBinary(ctx context.Context, localPath string, config AgentConfig) (string, error) {
	// 读取本地文件
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to read binary file: %w", err)
	}

	// 确定远程路径
	remoteDir := d.getRemoteAgentDir(config)
	remotePath := filepath.Join(remoteDir, "devssh-agent")

	// 创建远程目录
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remoteDir)
	if _, err := d.sshClient.RunCommand(mkdirCmd); err != nil {
		return "", fmt.Errorf("failed to create remote directory: %w", err)
	}

	// 上传文件
	// 这里使用base64编码传输，实际应该使用SCP或SFTP
	encodedData := fmt.Sprintf("echo '%s' | base64 -d > %s",
		strings.ReplaceAll(string(data), "'", "'\"'\"'"), remotePath)

	if _, err := d.sshClient.RunCommand(encodedData); err != nil {
		return "", fmt.Errorf("failed to upload binary: %w", err)
	}

	// 设置执行权限
	chmodCmd := fmt.Sprintf("chmod +x %s", remotePath)
	if _, err := d.sshClient.RunCommand(chmodCmd); err != nil {
		return "", fmt.Errorf("failed to set executable permission: %w", err)
	}

	return remotePath, nil
}

func (d *Deployer) createConfig(ctx context.Context, config AgentConfig) error {
	// 生成配置文件内容
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 确定远程配置路径
	remoteDir := d.getRemoteAgentDir(config)
	configPath := filepath.Join(remoteDir, "config.json")

	// 写入配置文件
	writeCmd := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", configPath, string(configJSON))
	if _, err := d.sshClient.RunCommand(writeCmd); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (d *Deployer) loadConfig(ctx context.Context) (AgentConfig, error) {
	// 确定远程配置路径
	remoteDir := d.getRemoteAgentDir(AgentConfig{})
	configPath := filepath.Join(remoteDir, "config.json")

	// 读取配置文件
	readCmd := fmt.Sprintf("cat %s", configPath)
	output, err := d.sshClient.RunCommand(readCmd)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("failed to read config: %w", err)
	}

	var config AgentConfig
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		return AgentConfig{}, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
}

func (d *Deployer) startAgent(ctx context.Context, binaryPath string, config AgentConfig) error {
	remoteDir := d.getRemoteAgentDir(config)
	logPath := filepath.Join(remoteDir, "agent.log")
	pidPath := filepath.Join(remoteDir, "agent.pid")

	// 构建启动命令
	startCmd := fmt.Sprintf(`
cd %s
nohup %s --config config.json > %s 2>&1 &
echo $! > %s
`, remoteDir, binaryPath, logPath, pidPath)

	if _, err := d.sshClient.RunCommand(startCmd); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	return nil
}

func (d *Deployer) stopAgent(ctx context.Context) error {
	remoteDir := d.getRemoteAgentDir(AgentConfig{})
	pidPath := filepath.Join(remoteDir, "agent.pid")

	// 读取PID文件
	readPidCmd := fmt.Sprintf("cat %s 2>/dev/null || true", pidPath)
	pidOutput, err := d.sshClient.RunCommand(readPidCmd)
	if err != nil {
		// 忽略错误，可能文件不存在
		return nil
	}

	pid := strings.TrimSpace(pidOutput)
	if pid == "" {
		return nil
	}

	// 停止进程
	stopCmd := fmt.Sprintf("kill %s 2>/dev/null || true", pid)
	if _, err := d.sshClient.RunCommand(stopCmd); err != nil {
		return fmt.Errorf("failed to stop agent: %w", err)
	}

	// 等待进程退出
	waitCmd := fmt.Sprintf("timeout 10 tail --pid=%s -f /dev/null 2>/dev/null || true", pid)
	d.sshClient.RunCommand(waitCmd)

	// 删除PID文件
	rmCmd := fmt.Sprintf("rm -f %s", pidPath)
	d.sshClient.RunCommand(rmCmd)

	return nil
}

func (d *Deployer) waitForAgent(ctx context.Context, config AgentConfig) error {
	// 等待最多30秒
	timeout := 30 * time.Second
	start := time.Now()

	for time.Since(start) < timeout {
		// 检查Agent是否响应
		checkCmd := fmt.Sprintf("curl -s -o /dev/null -w '%%{http_code}' http://%s:%d/health || true",
			config.BindAddress, config.ControlPort)

		output, err := d.sshClient.RunCommand(checkCmd)
		if err == nil && strings.TrimSpace(output) == "200" {
			return nil
		}

		// 等待1秒再试
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			continue
		}
	}

	return fmt.Errorf("agent failed to start within %v", timeout)
}

func (d *Deployer) cleanupFiles(ctx context.Context) error {
	remoteDir := d.getRemoteAgentDir(AgentConfig{})

	// 删除Agent目录
	rmCmd := fmt.Sprintf("rm -rf %s", remoteDir)
	if _, err := d.sshClient.RunCommand(rmCmd); err != nil {
		return fmt.Errorf("failed to cleanup files: %w", err)
	}

	return nil
}

func (d *Deployer) getRemoteAgentDir(config AgentConfig) string {
	// 使用配置中的工作目录，或默认目录
	if config.WorkDir != "" {
		return config.WorkDir
	}
	return "~/.devssh-agent"
}

// 辅助函数：检查远程命令是否可用
func (d *Deployer) checkCommand(ctx context.Context, command string) (bool, error) {
	checkCmd := fmt.Sprintf("command -v %s", command)
	_, err := d.sshClient.RunCommand(checkCmd)
	return err == nil, nil
}

// 辅助函数：获取远程系统信息
func (d *Deployer) getSystemInfo(ctx context.Context) (map[string]string, error) {
	info := make(map[string]string)

	// 获取发行版信息
	distroCmd := "cat /etc/os-release 2>/dev/null | grep -E '^(NAME|VERSION)=' || uname -s"
	distroOutput, err := d.sshClient.RunCommand(distroCmd)
	if err == nil {
		info["distro"] = strings.TrimSpace(distroOutput)
	}

	// 获取内核版本
	kernelCmd := "uname -r"
	kernelOutput, err := d.sshClient.RunCommand(kernelCmd)
	if err == nil {
		info["kernel"] = strings.TrimSpace(kernelOutput)
	}

	// 获取内存信息
	memCmd := "free -h 2>/dev/null | grep Mem | awk '{print $2}' || true"
	memOutput, err := d.sshClient.RunCommand(memCmd)
	if err == nil {
		info["memory"] = strings.TrimSpace(memOutput)
	}

	// 获取磁盘信息
	diskCmd := "df -h / | tail -1 | awk '{print $4}'"
	diskOutput, err := d.sshClient.RunCommand(diskCmd)
	if err == nil {
		info["disk_free"] = strings.TrimSpace(diskOutput)
	}

	return info, nil
}
