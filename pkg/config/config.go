package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devssh/pkg/ssh"
)

type HostConfig struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	KeyPath  string `json:"key_path,omitempty"`
}

type ConnectionConfig struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Port      string    `json:"port"`
	Username  string    `json:"username"`
	IDE       string    `json:"ide"`
	LocalPort int       `json:"local_port"`
	StartedAt time.Time `json:"started_at"`
	PID       int       `json:"pid,omitempty"`
}

type Config struct {
	Hosts       map[string]HostConfig       `json:"hosts"`
	Connections map[string]ConnectionConfig `json:"connections"`
}

func NewConfig() *Config {
	return &Config{
		Hosts:       make(map[string]HostConfig),
		Connections: make(map[string]ConnectionConfig),
	}
}

func (c *Config) Save() error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	// 确保目录存在
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

func (c *Config) Load() error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在，使用默认配置
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

func (c *Config) AddHost(host HostConfig) error {
	if host.Name == "" {
		return fmt.Errorf("host name is required")
	}

	c.Hosts[host.Name] = host
	return c.Save()
}

func (c *Config) RemoveHost(name string) error {
	if _, exists := c.Hosts[name]; !exists {
		return fmt.Errorf("host %s not found", name)
	}

	delete(c.Hosts, name)
	return c.Save()
}

func (c *Config) GetHost(name string) (HostConfig, bool) {
	host, exists := c.Hosts[name]
	return host, exists
}

func (c *Config) ListHosts() []HostConfig {
	hosts := make([]HostConfig, 0, len(c.Hosts))
	for _, host := range c.Hosts {
		hosts = append(hosts, host)
	}
	return hosts
}

// ImportSSHHosts 从SSH配置文件中导入主机
func (c *Config) ImportSSHHosts() (int, error) {
	parser := ssh.NewSSHConfigParser()
	sshHosts, err := parser.Parse()
	if err != nil {
		return 0, fmt.Errorf("failed to parse SSH config: %w", err)
	}

	imported := 0
	for hostName, sshHost := range sshHosts {
		// 跳过通配符主机
		if strings.Contains(hostName, "*") {
			continue
		}

		// 检查是否已存在
		if _, exists := c.Hosts[hostName]; !exists {
			hostConfig := HostConfig{
				Name:     hostName,
				Host:     sshHost.HostName,
				Port:     sshHost.Port,
				Username: sshHost.User,
				KeyPath:  sshHost.IdentityFile,
			}

			// 如果没有指定主机名，使用主机别名
			if hostConfig.Host == "" {
				hostConfig.Host = hostName
			}

			c.Hosts[hostName] = hostConfig
			imported++
		}
	}

	if imported > 0 {
		if err := c.Save(); err != nil {
			return imported, fmt.Errorf("imported %d hosts but failed to save config: %w", imported, err)
		}
	}

	return imported, nil
}

func (c *Config) AddConnection(conn ConnectionConfig) error {
	c.Connections[conn.ID] = conn
	return c.Save()
}

func (c *Config) RemoveConnection(id string) error {
	delete(c.Connections, id)
	return c.Save()
}

func (c *Config) GetConnection(id string) (ConnectionConfig, bool) {
	conn, exists := c.Connections[id]
	return conn, exists
}

func (c *Config) ListConnections() []ConnectionConfig {
	connections := make([]ConnectionConfig, 0, len(c.Connections))
	for _, conn := range c.Connections {
		connections = append(connections, conn)
	}
	return connections
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "devssh", "config.json"), nil
}

func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "devssh"), nil
}

func Load() (*Config, error) {
	config := NewConfig()
	if err := config.Load(); err != nil {
		return nil, err
	}
	return config, nil
}

func Save(cfg *Config) error {
	return cfg.Save()
}
