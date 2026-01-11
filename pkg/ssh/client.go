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
	}

	address := net.JoinHostPort(c.config.Host, c.config.Port)
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

	// 尝试私钥文件
	if c.config.KeyPath != "" {
		key, err := os.ReadFile(c.config.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
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
