package ssh

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

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
}

func (t *Tunnel) GetConfig() *TunnelConfig {
	return t.config
}

func NewTunnel(client *ssh.Client, config *TunnelConfig) *Tunnel {
	return &Tunnel{
		config: config,
		client: client,
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
		return
	}
	defer remoteConn.Close()

	// 双向转发数据
	done := make(chan struct{}, 2)

	go func() {
		_, _ = io.Copy(remoteConn, localConn)
		done <- struct{}{}
	}()

	go func() {
		_, _ = io.Copy(localConn, remoteConn)
		done <- struct{}{}
	}()

	<-done
	<-done
}

func ParsePortForward(forward string) (localPort, remotePort int, err error) {
	parts := strings.Split(forward, ":")

	switch len(parts) {
	case 1:
		// 格式: 8080 (本地和远程端口相同)
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid port: %w", err)
		}
		return port, port, nil

	case 2:
		// 格式: 8080:80 (本地端口:远程端口)
		localPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid local port: %w", err)
		}

		remotePort, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid remote port: %w", err)
		}

		return localPort, remotePort, nil

	default:
		return 0, 0, fmt.Errorf("invalid port forward format: %s", forward)
	}
}
