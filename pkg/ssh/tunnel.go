package ssh

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"devssh/pkg/logging"
	"github.com/loft-sh/log"
	"golang.org/x/crypto/ssh"
)

type TunnelConfig struct {
	LocalHost  string
	LocalPort  int
	RemoteHost string
	RemotePort int

	Dialer func() (*ssh.Client, error)
}

type Tunnel struct {
	config    *TunnelConfig
	client    *ssh.Client
	dedicated *ssh.Client
	listener  net.Listener
	closed    bool
	mu        sync.Mutex
	logger    log.Logger
}

func (t *Tunnel) GetConfig() *TunnelConfig {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.config
}

func (t *Tunnel) SetSSHClient(client *ssh.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.client = client
}

const (
	tcpKeepalivePeriod = 15 * time.Second
)

func NewTunnel(client *ssh.Client, config *TunnelConfig) *Tunnel {
	return &Tunnel{
		config: config,
		client: client,
		logger: logging.InitQuiet(),
	}
}

func NewTunnelWithLogger(client *ssh.Client, config *TunnelConfig, logger log.Logger) *Tunnel {
	return &Tunnel{
		config: config,
		client: client,
		logger: logger,
	}
}

func (t *Tunnel) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("tunnel is closed")
	}

	if t.config.Dialer != nil {
		dedicated, err := t.config.Dialer()
		if err != nil {
			return fmt.Errorf("failed to create dedicated SSH connection: %w", err)
		}
		t.dedicated = dedicated
	}

	localAddr := net.JoinHostPort(t.config.LocalHost, strconv.Itoa(t.config.LocalPort))
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		if t.dedicated != nil {
			t.dedicated.Close()
			t.dedicated = nil
		}
		return fmt.Errorf("failed to listen on local address: %w", err)
	}

	t.listener = listener

	go t.acceptConnections()

	return nil
}

func (t *Tunnel) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true

	if t.dedicated != nil {
		t.dedicated.Close()
		t.dedicated = nil
	}

	if t.listener != nil {
		return t.listener.Close()
	}

	return nil
}

func (t *Tunnel) acceptConnections() {
	for {
		localConn, err := t.listener.Accept()
		if err != nil {
			if t.closed {
				return
			}
			continue
		}

		go t.handleConnection(localConn)
	}
}

func optimizeTCPConn(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tcpConn.SetNoDelay(true)
	_ = tcpConn.SetKeepAlive(true)
	_ = tcpConn.SetKeepAlivePeriod(tcpKeepalivePeriod)
}

func (t *Tunnel) handleConnection(localConn net.Conn) {
	defer localConn.Close()

	optimizeTCPConn(localConn)

	sshClient := t.client
	if t.dedicated != nil {
		sshClient = t.dedicated
	}

	remoteAddr := net.JoinHostPort(t.config.RemoteHost, strconv.Itoa(t.config.RemotePort))
	remoteConn, err := sshClient.Dial("tcp", remoteAddr)
	if err != nil {
		t.logger.Errorf("Failed to dial remote %s: %v", remoteAddr, err)
		return
	}
	defer remoteConn.Close()

	optimizeTCPConn(remoteConn)

	done := make(chan struct{}, 2)

	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(remoteConn, localConn)
	}()

	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(localConn, remoteConn)
	}()

	<-done
	<-done
}


