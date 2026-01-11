package tunnel

import (
	"fmt"
	"sync"

	"github.com/sylens/project/DevSSH/pkg/ssh"
)

type TunnelManager struct {
	tunnels map[string]*ssh.Tunnel
	mu      sync.RWMutex
}

func NewTunnelManager() *TunnelManager {
	return &TunnelManager{
		tunnels: make(map[string]*ssh.Tunnel),
	}
}

func (m *TunnelManager) CreateTunnel(client *ssh.Client, localPort, remotePort int, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tunnels[name]; exists {
		return fmt.Errorf("tunnel %s already exists", name)
	}

	config := &ssh.TunnelConfig{
		LocalHost:  "127.0.0.1",
		LocalPort:  localPort,
		RemoteHost: "127.0.0.1",
		RemotePort: remotePort,
	}

	tunnel := ssh.NewTunnel(client.GetClient(), config)
	if err := tunnel.Start(); err != nil {
		return fmt.Errorf("failed to start tunnel: %w", err)
	}

	m.tunnels[name] = tunnel
	return nil
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

func CreatePortForwards(client *ssh.Client, configs []ForwardConfig, manager *TunnelManager) error {
	for i, config := range configs {
		name := fmt.Sprintf("tunnel-%d", i)

		if config.AutoDetect {
			// 自动检测并转发端口
			scanner := NewPortScanner(client)
			ports, err := scanner.DetectWebServices()
			if err != nil {
				return fmt.Errorf("failed to detect web services: %w", err)
			}

			for _, portInfo := range ports {
				tunnelName := fmt.Sprintf("auto-%d", portInfo.Port)
				if err := manager.CreateTunnel(client, portInfo.Port, portInfo.Port, tunnelName); err != nil {
					return fmt.Errorf("failed to create auto tunnel for port %d: %w", portInfo.Port, err)
				}
				fmt.Printf("Auto-forwarding port %d (%s)\n", portInfo.Port, portInfo.Service)
			}
		} else {
			// 手动指定端口转发
			if err := manager.CreateTunnel(client, config.LocalPort, config.RemotePort, name); err != nil {
				return fmt.Errorf("failed to create tunnel for port %d->%d: %w", config.LocalPort, config.RemotePort, err)
			}
			fmt.Printf("Forwarding local port %d to remote port %d\n", config.LocalPort, config.RemotePort)
		}
	}

	return nil
}
