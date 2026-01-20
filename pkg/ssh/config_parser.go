package ssh

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SSHHostConfig 表示SSH配置文件中的主机配置
type SSHHostConfig struct {
	Host         string
	HostName     string
	User         string
	Port         string
	IdentityFile string
	ProxyJump    string
	ForwardAgent string
}

// SSHConfigParser 用于解析SSH配置文件
type SSHConfigParser struct {
	configPath string
}

// NewSSHConfigParser 创建新的SSH配置文件解析器
func NewSSHConfigParser() *SSHConfigParser {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	return &SSHConfigParser{
		configPath: filepath.Join(homeDir, ".ssh", "config"),
	}
}

// WithConfigPath 设置自定义配置文件路径
func (p *SSHConfigParser) WithConfigPath(path string) *SSHConfigParser {
	p.configPath = path
	return p
}

// Parse 解析SSH配置文件
func (p *SSHConfigParser) Parse() (map[string]*SSHHostConfig, error) {
	file, err := os.Open(p.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*SSHHostConfig), nil
		}
		return nil, fmt.Errorf("failed to open SSH config file: %w", err)
	}
	defer file.Close()

	hosts := make(map[string]*SSHHostConfig)
	var currentHost *SSHHostConfig
	var currentHostNames []string

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 处理续行
		for strings.HasSuffix(line, "\\") {
			line = strings.TrimSuffix(line, "\\")
			if !scanner.Scan() {
				break
			}
			lineNum++
			nextLine := strings.TrimSpace(scanner.Text())
			line += " " + nextLine
		}

		// 分割键值对
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")

		switch key {
		case "host":
			// 保存上一个主机的配置（过滤特殊主机）
			if currentHost != nil {
				for _, hostName := range currentHostNames {
					// 跳过特殊主机模式
					if !isSpecialHostPattern(hostName) {
						hosts[hostName] = currentHost
					}
				}
			}

			// 创建新的主机配置
			currentHost = &SSHHostConfig{
				Port: "22", // 默认端口
			}
			currentHostNames = strings.Fields(value)

			// 设置主机别名
			if len(currentHostNames) > 0 {
				currentHost.Host = currentHostNames[0]
			}

		case "hostname":
			if currentHost != nil {
				currentHost.HostName = value
			}

		case "user":
			if currentHost != nil {
				currentHost.User = value
			}

		case "port":
			if currentHost != nil {
				// 验证端口号
				if port, err := strconv.Atoi(value); err == nil && port > 0 && port <= 65535 {
					currentHost.Port = value
				}
			}

		case "identityfile":
			if currentHost != nil {
				// 展开波浪号路径
				if strings.HasPrefix(value, "~") {
					homeDir, err := os.UserHomeDir()
					if err == nil {
						value = filepath.Join(homeDir, value[1:])
					}
				}
				currentHost.IdentityFile = value
			}

		case "proxyjump":
			if currentHost != nil {
				currentHost.ProxyJump = value
			}

		case "forwardagent":
			if currentHost != nil {
				currentHost.ForwardAgent = value
			}
		}
	}

	// 保存最后一个主机的配置（过滤特殊主机）
	if currentHost != nil {
		for _, hostName := range currentHostNames {
			// 跳过特殊主机模式
			if !isSpecialHostPattern(hostName) {
				hosts[hostName] = currentHost
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading SSH config file: %w", err)
	}

	return hosts, nil
}

// GetHost 获取指定主机的配置
func (p *SSHConfigParser) GetHost(hostName string) (*SSHHostConfig, error) {
	// 首先检查是否是特殊主机模式
	if isSpecialHostPattern(hostName) {
		return nil, fmt.Errorf("host %s is a special pattern (contains wildcards) and cannot be used for direct connection", hostName)
	}

	hosts, err := p.Parse()
	if err != nil {
		return nil, err
	}

	if host, exists := hosts[hostName]; exists {
		return host, nil
	}

	return nil, fmt.Errorf("host %s not found in SSH config", hostName)
}

// ListHosts 列出所有配置的主机
func (p *SSHConfigParser) ListHosts() ([]string, error) {
	hosts, err := p.Parse()
	if err != nil {
		return nil, err
	}

	hostNames := make([]string, 0, len(hosts))
	for hostName := range hosts {
		hostNames = append(hostNames, hostName)
	}

	return hostNames, nil
}

// isSpecialHostPattern 检查是否是特殊主机模式
// 特殊模式包括：*、?、! 以及包含这些通配符的模式
func isSpecialHostPattern(hostName string) bool {
	// 检查是否包含通配符
	if strings.ContainsAny(hostName, "*?!") {
		return true
	}

	// 检查是否是单个字符的匹配模式
	// 例如：Host ? 或 Host ???
	if len(hostName) == 1 && hostName == "?" {
		return true
	}

	// 检查是否以!开头（排除模式）
	if strings.HasPrefix(hostName, "!") {
		return true
	}

	return false
}

// GetHostConfigForSSH 将SSHHostConfig转换为SSH Config
func (h *SSHHostConfig) GetHostConfigForSSH() *Config {
	config := &Config{
		Host:     h.HostName,
		Port:     h.Port,
		Username: h.User,
		KeyPath:  h.IdentityFile,
		Timeout:  30 * time.Second,
	}

	// 如果没有指定主机名，使用主机别名
	if config.Host == "" {
		config.Host = h.Host
	}

	return config
}
