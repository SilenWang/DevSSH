package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"devssh/pkg/logging"
	"github.com/loft-sh/log"
	"golang.org/x/crypto/ssh"
)

const (
	defaultKeepaliveInterval = 30 * time.Second
	healthCheckTimeout       = 10 * time.Second
)

type Config struct {
	Host              string
	Port              string
	Username          string
	KeyPath           string
	Password          string
	Timeout           time.Duration
	KeepaliveInterval time.Duration
}

type Client struct {
	config *Config
	client *ssh.Client
	logger log.Logger

	mu              sync.Mutex
	connected       atomic.Bool
	keepaliveStop   chan struct{}
	keepaliveWg     sync.WaitGroup
	onReconnect     func()
	healthCheckFn   func() bool
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

func (c *Client) SetReconnectHandler(handler func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onReconnect = handler
}

func (c *Client) EnableKeepalive() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.config.KeepaliveInterval <= 0 {
		c.config.KeepaliveInterval = defaultKeepaliveInterval
	}
}

func (c *Client) DisableKeepalive() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keepaliveStop = nil
	c.config.KeepaliveInterval = 0
}

func (c *Client) SetHealthCheckFn(fn func() bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthCheckFn = fn
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
		if overrideConfig.KeepaliveInterval > 0 {
			config.KeepaliveInterval = overrideConfig.KeepaliveInterval
		}
	}

	return NewClientWithLogger(config, logger), nil
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	client, err := c.dial()
	if err != nil {
		return err
	}

	c.client = client
	c.connected.Store(true)
	c.logger.Infof("SSH connection established successfully")

	// 启动心跳检测
	interval := c.config.KeepaliveInterval
	if interval > 0 {
		c.startKeepaliveLocked(interval)
	}

	return nil
}

func (c *Client) dial() (*ssh.Client, error) {
	authMethods, err := c.getAuthMethods()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth methods: %w", err)
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

	if c.config.Password != "" {
		c.logger.Infof("Using password authentication (password provided)")
	}
	if c.config.KeyPath != "" {
		c.logger.Infof("Using private key: %s", c.config.KeyPath)
	}

	tcpConn, tcpErr := net.DialTimeout("tcp", address, c.config.Timeout)
	if tcpErr != nil {
		return nil, fmt.Errorf("TCP connection failed: %w", tcpErr)
	}
	tcpConn.Close()
	c.logger.Infof("TCP connection successful, attempting SSH handshake...")

	cli, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial SSH: %w", err)
	}
	return cli, nil
}

func (c *Client) startKeepaliveLocked(interval time.Duration) {
	if c.keepaliveStop != nil {
		return
	}
	c.keepaliveStop = make(chan struct{})
	c.keepaliveWg.Add(1)
	go c.keepaliveLoop(interval)
}

func (c *Client) keepaliveLoop(interval time.Duration) {
	defer c.keepaliveWg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.keepaliveStop:
			return
		case <-ticker.C:
			c.mu.Lock()
			currentClient := c.client
			c.mu.Unlock()

			if currentClient == nil {
				continue
			}

			if !c.checkHealth(currentClient) {
				c.logger.Warnf("Connection health check failed, attempting reconnect...")
				if err := c.reconnect(); err != nil {
					c.logger.Errorf("Reconnect failed: %v", err)
					continue
				}
				c.logger.Infof("Reconnected successfully")

				c.mu.Lock()
				handler := c.onReconnect
				c.mu.Unlock()

				if handler != nil {
					handler()
				}
			}
		}
	}
}

func (c *Client) checkHealth(cli *ssh.Client) bool {
	if c.healthCheckFn != nil {
		return c.healthCheckFn()
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := cli.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()

	select {
	case err := <-done:
		return err == nil
	case <-time.After(healthCheckTimeout):
		return false
	}
}

func (c *Client) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldClient := c.client
	c.client = nil
	c.connected.Store(false)

	if oldClient != nil {
		oldClient.Close()
	}

	newClient, err := c.dial()
	if err != nil {
		return fmt.Errorf("reconnect failed: %w", err)
	}

	c.client = newClient
	c.connected.Store(true)
	return nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.keepaliveStop != nil {
		close(c.keepaliveStop)
		c.keepaliveStop = nil
	}
	c.keepaliveWg.Wait()

	c.connected.Store(false)
	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		return err
	}
	return nil
}

func (c *Client) getClient() *ssh.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

func (c *Client) RunCommand(cmd string) (string, error) {
	cli := c.getClient()
	if cli == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := cli.NewSession()
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
	cli := c.getClient()
	if cli == nil {
		return fmt.Errorf("not connected")
	}

	session, err := cli.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	return session.Run(cmd)
}

func (c *Client) NewSession() (*ssh.Session, error) {
	cli := c.getClient()
	if cli == nil {
		return nil, fmt.Errorf("not connected")
	}
	return cli.NewSession()
}

func (c *Client) getAuthMethods() ([]ssh.AuthMethod, error) {
	var authMethods []ssh.AuthMethod

	// 如果提供了密码，优先尝试密码认证
	if c.config.Password != "" {
		authMethods = append(authMethods, ssh.Password(c.config.Password))
		c.logger.Infof("Added password authentication method")
	}

	// 尝试配置文件中指定的私钥文件（来自 --key 或 SSH config 的 IdentityFile）
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
						signer, innerErr := ssh.ParsePrivateKeyWithPassphrase(key, []byte(c.config.Password))
						if innerErr == nil {
							c.addKeySigner(&authMethods, signer, "with passphrase")
							c.logger.Infof("Added private key authentication (with passphrase) from config: %s", c.config.KeyPath)
						} else {
							c.logger.Warnf("Failed to parse private key (even with passphrase): %v", innerErr)
						}
					} else {
						c.logger.Warnf("Failed to parse private key (may be passphrase protected): %v", err)
					}
				} else {
					c.addKeySigner(&authMethods, signer, "")
					c.logger.Infof("Added private key authentication from config: %s (type: %s)", c.config.KeyPath, signer.PublicKey().Type())
				}
			}
		} else {
			c.logger.Warnf("Private key file not found: %s", c.config.KeyPath)
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods available")
	}

	c.logger.Infof("Total authentication methods: %d", len(authMethods))
	return authMethods, nil
}

// addKeySigner adds a signer to the auth methods.
// For RSA keys, it adds both the original signer and a wrapped signer with
// SHA-2 signature algorithms, covering both modern and legacy SSH servers.
func (c *Client) addKeySigner(authMethods *[]ssh.AuthMethod, signer ssh.Signer, label string) {
	keyType := signer.PublicKey().Type()
	c.logger.Infof("Adding key signer: type=%s label=%s", keyType, label)

	// Always add the signer as-is (default algorithm)
	*authMethods = append(*authMethods, ssh.PublicKeys(signer))
	c.logger.Debugf("Added default signer with algorithm: %s", keyType)

	// For RSA keys, also add wrapped signers for SHA-2 algorithm support
	if keyType == "ssh-rsa" {
		algSigner, ok := signer.(ssh.AlgorithmSigner)
		if !ok {
			c.logger.Debugf("RSA signer does not implement AlgorithmSigner, skipping algorithm wrapping")
			return
		}

		// Add wrapped signer with all RSA algorithms (SHA-2 preferred over SHA-1)
		algos := []string{
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSASHA512,
			ssh.KeyAlgoRSA,
		}
		wrapped, err := ssh.NewSignerWithAlgorithms(algSigner, algos)
		if err != nil {
			c.logger.Warnf("Failed to wrap RSA signer with algorithms: %v", err)
		} else {
			*authMethods = append(*authMethods, ssh.PublicKeys(wrapped))
			c.logger.Infof("Added wrapped RSA signer with algorithms: rsa-sha2-256 > rsa-sha2-512 > ssh-rsa")
		}
	}
}

func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

func (c *Client) GetClient() *ssh.Client {
	return c.getClient()
}

func (c *Client) SetLogger(logger log.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger = logger
}

func (c *Client) GetConfig() *Config {
	return c.config
}

func (c *Client) NewSCPClient() *SCPClient {
	return NewSCPClient(c)
}
