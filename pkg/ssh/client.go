package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"devssh/pkg/logging"
	"github.com/loft-sh/log"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type Config struct {
	Host     string
	Port     string
	Username string
	KeyPath  string
	Password string
	Timeout  time.Duration
}

type Client struct {
	config *Config
	client *ssh.Client
	logger log.Logger
}

func NewClient(config *Config) *Client {
	logger := logging.InitQuiet()
	return &Client{
		config: config,
		logger: logger,
	}
}

func NewClientWithLogger(config *Config, logger log.Logger) *Client {
	return &Client{
		config: config,
		logger: logger,
	}
}

// NewClientFromSSHConfig 从SSH配置文件创建客户端
func NewClientFromSSHConfig(hostName string, overrideConfig *Config) (*Client, error) {
	logger := logging.InitQuiet()
	return NewClientFromSSHConfigWithLogger(hostName, overrideConfig, logger)
}

// NewClientFromSSHConfigWithLogger 从SSH配置文件创建客户端（带logger）
func NewClientFromSSHConfigWithLogger(hostName string, overrideConfig *Config, logger log.Logger) (*Client, error) {
	parser := NewSSHConfigParser()
	sshHostConfig, err := parser.GetHost(hostName)
	if err != nil {
		return nil, fmt.Errorf("failed to get host config from SSH config: %w", err)
	}

	config := sshHostConfig.GetHostConfigForSSH()

	// 使用命令行参数覆盖配置文件中的设置
	if overrideConfig != nil {
		if overrideConfig.Username != "" {
			config.Username = overrideConfig.Username
		}
		if overrideConfig.Port != "" {
			config.Port = overrideConfig.Port
		}
		if overrideConfig.KeyPath != "" {
			config.KeyPath = overrideConfig.KeyPath
		}
		if overrideConfig.Password != "" {
			config.Password = overrideConfig.Password
		}
		if overrideConfig.Timeout > 0 {
			config.Timeout = overrideConfig.Timeout
		}
	}

	return NewClientWithLogger(config, logger), nil
}

func (c *Client) Connect() error {
	authMethods, err := c.getAuthMethods()
	if err != nil {
		return fmt.Errorf("failed to get auth methods: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            c.config.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.config.Timeout,
		Config: ssh.Config{
			Ciphers: []string{
				"aes128-ctr", "aes192-ctr", "aes256-ctr",
				"aes128-gcm@openssh.com", "aes256-gcm@openssh.com",
				"chacha20-poly1305@openssh.com",
			},
		},
		ClientVersion: "SSH-2.0-OpenSSH_9.2",
	}

	address := net.JoinHostPort(c.config.Host, c.config.Port)
	c.logger.Infof("Attempting to connect to %s as user '%s' with timeout %v", address, c.config.Username, c.config.Timeout)

	// 显示使用的认证方法
	if c.config.Password != "" {
		c.logger.Infof("Using password authentication (password provided)")
	}
	if c.config.KeyPath != "" {
		c.logger.Infof("Using private key: %s", c.config.KeyPath)
	}

	// 先测试TCP连接
	tcpConn, tcpErr := net.DialTimeout("tcp", address, c.config.Timeout)
	if tcpErr != nil {
		return fmt.Errorf("TCP connection failed: %w", tcpErr)
	}
	tcpConn.Close()
	c.logger.Infof("TCP connection successful, attempting SSH handshake...")

	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to dial SSH: %w", err)
	}

	c.client = client
	c.logger.Infof("SSH connection established successfully")
	return nil
}

func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *Client) RunCommand(cmd string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}

	return string(output), nil
}

func (c *Client) RunCommandWithOutput(cmd string, stdout, stderr io.Writer) error {
	if c.client == nil {
		return fmt.Errorf("not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	return session.Run(cmd)
}

func (c *Client) NewSession() (*ssh.Session, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	return c.client.NewSession()
}

func (c *Client) getAuthMethods() ([]ssh.AuthMethod, error) {
	var authMethods []ssh.AuthMethod

	// 如果提供了密码，优先尝试密码认证
	if c.config.Password != "" {
		authMethods = append(authMethods, ssh.Password(c.config.Password))
		c.logger.Infof("Added password authentication method")
	}

	// 尝试 SSH agent
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		authMethods = append(authMethods, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
		c.logger.Infof("Added SSH agent authentication method")
	}

	// 尝试配置文件中指定的私钥文件
	if c.config.KeyPath != "" {
		if _, err := os.Stat(c.config.KeyPath); err == nil {
			key, err := os.ReadFile(c.config.KeyPath)
			if err != nil {
				c.logger.Warnf("Failed to read private key from config: %v", err)
			} else {
				signer, err := ssh.ParsePrivateKey(key)
				if err != nil {
					// 私钥可能有密码保护，尝试使用密码解析
					if c.config.Password != "" {
						signer, err := ssh.ParsePrivateKeyWithPassphrase(key, []byte(c.config.Password))
						if err == nil {
							authMethods = append(authMethods, ssh.PublicKeys(signer))
							c.logger.Infof("Added private key authentication (with passphrase) from config: %s", c.config.KeyPath)
						} else {
							c.logger.Warnf("Failed to parse private key (even with passphrase): %v", err)
						}
					} else {
						c.logger.Warnf("Failed to parse private key (may be passphrase protected): %v", err)
					}
				} else {
					authMethods = append(authMethods, ssh.PublicKeys(signer))
					c.logger.Infof("Added private key authentication from config: %s", c.config.KeyPath)
				}
			}
		} else {
			c.logger.Warnf("Private key file not found: %s", c.config.KeyPath)
		}
	}

	// 尝试默认的私钥位置
	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultKeyPaths := []string{
			filepath.Join(homeDir, ".ssh", "id_rsa"),
			filepath.Join(homeDir, ".ssh", "id_ed25519"),
			filepath.Join(homeDir, ".ssh", "id_ecdsa"),
		}

		for _, keyPath := range defaultKeyPaths {
			if _, err := os.Stat(keyPath); err == nil {
				key, err := os.ReadFile(keyPath)
				if err != nil {
					continue
				}

				signer, err := ssh.ParsePrivateKey(key)
				if err != nil {
					continue
				}

				authMethods = append(authMethods, ssh.PublicKeys(signer))
				c.logger.Infof("Added default private key authentication: %s", keyPath)
				break
			}
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods available")
	}

	c.logger.Infof("Total authentication methods: %d", len(authMethods))
	return authMethods, nil
}

func (c *Client) IsConnected() bool {
	return c.client != nil
}

func (c *Client) GetClient() *ssh.Client {
	return c.client
}

// SetLogger 设置logger
func (c *Client) SetLogger(logger log.Logger) {
	c.logger = logger
}

func (c *Client) GetConfig() *Config {
	return c.config
}

// NewSCPClient 创建SCP客户端
func (c *Client) NewSCPClient() *SCPClient {
	return NewSCPClient(c)
}
