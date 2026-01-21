package tunnel

import (
	"fmt"
	"io"
	"sync"

	"devssh/pkg/ssh"
	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
)

type TunnelManager struct {
	tunnels map[string]*ssh.Tunnel
	mu      sync.RWMutex
	logger  log.Logger
}

func NewTunnelManager() *TunnelManager {
	// 创建一个不输出任何内容的logger
	logger := log.NewStreamLogger(io.Discard, io.Discard, logrus.InfoLevel)
	return &TunnelManager{
		tunnels: make(map[string]*ssh.Tunnel),
		logger:  logger,
	}
}

func NewTunnelManagerWithLogger(logger log.Logger) *TunnelManager {
	return &TunnelManager{
		tunnels: make(map[string]*ssh.Tunnel),
		logger:  logger,
	}
}

func (m *TunnelManager) CreateTunnel(client *ssh.Client, localPort, remotePort int, name string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tunnels[name]; exists {
		return 0, fmt.Errorf("tunnel %s already exists", name)
	}

	// 记录日志的函数
	logFunc := func(msg string) {
		m.logger.Info(msg)
	}

	// 查找可用端口
	actualPort, err := FindAvailablePort(localPort, logFunc)
	if err != nil {
		return 0, fmt.Errorf("failed to find available port for tunnel %s: %w", name, err)
	}

	// 如果端口有变化，记录最终结果
	if actualPort != localPort {
		m.logger.Infof("Local Port %d was occupied, automatically switch to port %d", localPort, actualPort)
	}

	config := &ssh.TunnelConfig{
		LocalHost:  "127.0.0.1",
		LocalPort:  actualPort,
		RemoteHost: "127.0.0.1",
		RemotePort: remotePort,
	}

	tunnel := ssh.NewTunnel(client.GetClient(), config)
	if err := tunnel.Start(); err != nil {
		return 0, fmt.Errorf("failed to start tunnel on port %d: %w", actualPort, err)
	}

	m.tunnels[name] = tunnel
	return actualPort, nil
}

func (m *TunnelManager) StopTunnel(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tunnel, exists := m.tunnels[name]
	if !exists {
		return fmt.Errorf("tunnel %s not found", name)
	}

	if err := tunnel.Stop(); err != nil {
		return fmt.Errorf("failed to stop tunnel: %w", err)
	}

	delete(m.tunnels, name)
	return nil
}

func (m *TunnelManager) StopAllTunnels() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error
	for name, tunnel := range m.tunnels {
		if err := tunnel.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop tunnel %s: %w", name, err))
		}
	}

	m.tunnels = make(map[string]*ssh.Tunnel)

	if len(errors) > 0 {
		return fmt.Errorf("multiple errors occurred: %v", errors)
	}

	return nil
}

func (m *TunnelManager) ListTunnels() map[string]struct {
	LocalPort  int
	RemotePort int
} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]struct {
		LocalPort  int
		RemotePort int
	})

	for name, tunnel := range m.tunnels {
		config := tunnel.GetConfig()
		result[name] = struct {
			LocalPort  int
			RemotePort int
		}{
			LocalPort:  config.LocalPort,
			RemotePort: config.RemotePort,
		}
	}

	return result
}

func (m *TunnelManager) HasTunnel(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.tunnels[name]
	return exists
}

func (m *TunnelManager) GetTunnel(name string) (*ssh.Tunnel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tunnel, exists := m.tunnels[name]
	return tunnel, exists
}

type ForwardConfig struct {
	LocalPort  int
	RemotePort int
	AutoDetect bool
}

type PortForwardResult struct {
	Name       string
	LocalPort  int
	RemotePort int
	ActualPort int
}

func CreatePortForwards(client *ssh.Client, configs []ForwardConfig, manager *TunnelManager) ([]PortForwardResult, error) {
	var results []PortForwardResult

	for i, config := range configs {
		name := fmt.Sprintf("tunnel-%d", i)

		if config.AutoDetect {
			// 自动检测并转发端口
			scanner := NewPortScanner(client)
			ports, err := scanner.DetectWebServices()
			if err != nil {
				return nil, fmt.Errorf("failed to detect web services: %w", err)
			}

			for _, portInfo := range ports {
				tunnelName := fmt.Sprintf("auto-%d", portInfo.Port)
				actualPort, err := manager.CreateTunnel(client, portInfo.Port, portInfo.Port, tunnelName)
				if err != nil {
					return nil, fmt.Errorf("failed to create auto tunnel for port %d: %w", portInfo.Port, err)
				}
				manager.logger.Infof("Auto-forwarding port %d (%s)", portInfo.Port, portInfo.Service)

				results = append(results, PortForwardResult{
					Name:       tunnelName,
					LocalPort:  portInfo.Port,
					RemotePort: portInfo.Port,
					ActualPort: actualPort,
				})
			}
		} else {
			// 手动指定端口转发
			actualPort, err := manager.CreateTunnel(client, config.LocalPort, config.RemotePort, name)
			if err != nil {
				return nil, fmt.Errorf("failed to create tunnel for port %d->%d: %w", config.LocalPort, config.RemotePort, err)
			}
			manager.logger.Infof("Forwarding local port %d to remote port %d", config.LocalPort, config.RemotePort)

			results = append(results, PortForwardResult{
				Name:       name,
				LocalPort:  config.LocalPort,
				RemotePort: config.RemotePort,
				ActualPort: actualPort,
			})
		}
	}

	return results, nil
}
