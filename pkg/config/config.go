package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Env override keys for testing without GitHub releases
const (
	EnvDevSSHDownloadURL = "DEVSSH_DEVSSH_DOWNLOAD_URL"
	EnvVSCodeDownloadURL = "DEVSSH_VSCODE_DOWNLOAD_URL"
)

const (
	DevSSHDownloadURL = "https://github.com/SilenWang/DevSSH/releases/download/{{version}}/devssh-{{version}}-{{os}}-{{arch}}"
)

type HostConfig struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	KeyPath  string `json:"key_path,omitempty"`
}

type Config struct {
	Hosts map[string]HostConfig `json:"hosts"`
}

func NewConfig() *Config {
	return &Config{
		Hosts: make(map[string]HostConfig),
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

func GetDevSSHDownloadURL(version, goos, arch string) string {
	template := DevSSHDownloadURL
	if envURL := os.Getenv(EnvDevSSHDownloadURL); envURL != "" {
		template = envURL
	}

	replacer := strings.NewReplacer(
		"{{version}}", version,
		"{{os}}", goos,
		"{{arch}}", arch,
	)

	url := replacer.Replace(template)
	return url
}


