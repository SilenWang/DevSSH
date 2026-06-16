package ssh

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"

	"devssh/pkg/logging"
	"github.com/loft-sh/log"
	"golang.org/x/crypto/ssh"
)

type TunnelConfig struct {
	LocalHost  string
	LocalPort  int
	RemoteHost string
	RemotePort int
}

type Tunnel struct {
	config   *TunnelConfig
	client   *ssh.Client
	listener net.Listener
	closed   bool
	mu       sync.Mutex
	logger   log.Logger
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
	copyBufferSize = 256 * 1024
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

	localAddr := net.JoinHostPort(t.config.LocalHost, strconv.Itoa(t.config.LocalPort))
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
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

func (t *Tunnel) handleConnection(localConn net.Conn) {
	defer localConn.Close()

	remoteAddr := net.JoinHostPort(t.config.RemoteHost, strconv.Itoa(t.config.RemotePort))
	remoteConn, err := t.client.Dial("tcp", remoteAddr)
	if err != nil {
		t.logger.Errorf("Failed to dial remote %s: %v", remoteAddr, err)
		return
	}
	defer remoteConn.Close()

	done := make(chan error, 2)

	go func() {
		buf := make([]byte, copyBufferSize)
		_, err := io.CopyBuffer(remoteConn, localConn, buf)
		done <- err
	}()

	go func() {
		buf := make([]byte, copyBufferSize)
		_, err := io.CopyBuffer(localConn, remoteConn, buf)
		done <- err
	}()

	err1 := <-done
	err2 := <-done

	if err1 != nil {
		t.logger.Debugf("Forward local->remote error: %v", err1)
	}
	if err2 != nil {
		t.logger.Debugf("Forward remote->local error: %v", err2)
	}
}


