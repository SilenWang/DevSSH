package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

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
}

func NewClient(config *Config) *Client {
	return &Client{
		config: config,
	}
}

// NewClientFromSSHConfig 从SSH配置文件创建客户端
func NewClientFromSSHConfig(hostName string) (*Client, error) {
	parser := NewSSHConfigParser()
	sshHostConfig, err := parser.GetHost(hostName)
	if err != nil {
		return nil, fmt.Errorf("failed to get host config from SSH config: %w", err)
	}

	config := sshHostConfig.GetHostConfigForSSH()
	return NewClient(config), nil
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
	fmt.Printf("Attempting to connect to %s with timeout %v\n", address, c.config.Timeout)

	// 先测试TCP连接
	tcpConn, tcpErr := net.DialTimeout("tcp", address, c.config.Timeout)
	if tcpErr != nil {
		return fmt.Errorf("TCP connection failed: %w", tcpErr)
	}
	tcpConn.Close()
	fmt.Println("TCP connection successful, attempting SSH handshake...")

	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to dial SSH: %w", err)
	}

	c.client = client
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

	// 尝试 SSH agent
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		authMethods = append(authMethods, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}

	// 首先尝试配置文件中指定的私钥文件
	if c.config.KeyPath != "" {
		key, err := os.ReadFile(c.config.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key from config: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key from config: %w", err)
		}

		authMethods = append(authMethods, ssh.PublicKeys(signer))
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
				break
			}
		}
	}

	// 尝试密码认证
	if c.config.Password != "" {
		authMethods = append(authMethods, ssh.Password(c.config.Password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods available")
	}

	return authMethods, nil
}

func (c *Client) IsConnected() bool {
	return c.client != nil
}

func (c *Client) GetClient() *ssh.Client {
	return c.client
}

func (c *Client) GetConfig() *Config {
	return c.config
}
