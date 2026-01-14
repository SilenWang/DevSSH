package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"devssh/pkg/ssh"
)

// DefaultFactory 默认工厂实现
type DefaultFactory struct {
	binaryMgr *BinaryManager
}

// NewDefaultFactory 创建默认工厂
func NewDefaultFactory() *DefaultFactory {
	return &DefaultFactory{
		binaryMgr: NewBinaryManager(""),
	}
}

// CreateAgent 创建Agent实例
func (f *DefaultFactory) CreateAgent(config AgentConfig) (Agent, error) {
	// 创建HTTP服务器（用于测试）
	_, err := NewHTTPServer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	// HTTPServer实现了Server接口，需要包装成Agent
	// 这里简化处理，实际应该创建一个包装器
	return nil, fmt.Errorf("CreateAgent not implemented for server, use CreateServer instead")
}

// CreateClient 创建客户端
func (f *DefaultFactory) CreateClient(config AgentConfig) (Client, error) {
	// 创建HTTP客户端
	client, err := NewHTTPClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return client, nil
}

// CreateServer 创建服务器
func (f *DefaultFactory) CreateServer(config AgentConfig) (Server, error) {
	// 创建HTTP服务器
	server, err := NewHTTPServer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	return server, nil
}

// CreateAgentFromSSH 从SSH客户端创建Agent
func (f *DefaultFactory) CreateAgentFromSSH(sshClient interface{}, config AgentConfig) (Agent, error) {
	// 类型断言
	client, ok := sshClient.(*ssh.Client)
	if !ok {
		return nil, fmt.Errorf("invalid SSH client type")
	}

	// 创建SSH客户端包装器
	sshWrapper, err := NewSSHClient(client, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client wrapper: %w", err)
	}

	// SSHClient实现了Client接口，需要包装成Agent
	// 这里简化处理，返回Client（Client也实现了Agent接口）
	return sshWrapper, nil
}

// Manager Agent管理器
type Manager struct {
	factory Factory
	agents  map[string]Agent
	clients map[string]Client
	servers map[string]Server
	mu      sync.RWMutex
}

// NewManager 创建Agent管理器
func NewManager(factory Factory) *Manager {
	if factory == nil {
		factory = NewDefaultFactory()
	}

	return &Manager{
		factory: factory,
		agents:  make(map[string]Agent),
		clients: make(map[string]Client),
		servers: make(map[string]Server),
	}
}

// CreateAgent 创建并注册Agent
func (m *Manager) CreateAgent(config AgentConfig) (Agent, error) {
	agent, err := m.factory.CreateAgent(config)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.agents[config.Host] = agent
	m.mu.Unlock()

	return agent, nil
}

// CreateClient 创建并注册客户端
func (m *Manager) CreateClient(config AgentConfig) (Client, error) {
	client, err := m.factory.CreateClient(config)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.clients[config.Host] = client
	m.mu.Unlock()

	return client, nil
}

// CreateServer 创建并注册服务器
func (m *Manager) CreateServer(config AgentConfig) (Server, error) {
	server, err := m.factory.CreateServer(config)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.servers[config.Host] = server
	m.mu.Unlock()

	return server, nil
}

// GetAgent 获取Agent
func (m *Manager) GetAgent(host string) (Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, exists := m.agents[host]
	return agent, exists
}

// GetClient 获取客户端
func (m *Manager) GetClient(host string) (Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, exists := m.clients[host]
	return client, exists
}

// GetServer 获取服务器
func (m *Manager) GetServer(host string) (Server, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	server, exists := m.servers[host]
	return server, exists
}

// RemoveAgent 移除Agent
func (m *Manager) RemoveAgent(host string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, exists := m.agents[host]; exists {
		if err := agent.Close(); err != nil {
			return err
		}
		delete(m.agents, host)
	}

	return nil
}

// RemoveClient 移除客户端
func (m *Manager) RemoveClient(host string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if client, exists := m.clients[host]; exists {
		if err := client.Close(); err != nil {
			return err
		}
		delete(m.clients, host)
	}

	return nil
}

// RemoveServer 移除服务器
func (m *Manager) RemoveServer(host string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if server, exists := m.servers[host]; exists {
		// 服务器没有Close方法，需要停止
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Stop(ctx); err != nil {
			return err
		}
		delete(m.servers, host)
	}

	return nil
}

// ListAgents 列出所有Agent
func (m *Manager) ListAgents() []AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var infos []AgentInfo
	for host, agent := range m.agents {
		infos = append(infos, AgentInfo{
			ID:      host,
			Status:  agent.Status(),
			Version: agent.Version(),
			Config: AgentConfig{
				Host: host,
			},
		})
	}

	return infos
}

// ListClients 列出所有客户端
func (m *Manager) ListClients() []ClientInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var infos []ClientInfo
	for host, client := range m.clients {
		infos = append(infos, ClientInfo{
			ID:        host,
			AgentID:   client.ID(),
			Status:    client.Status(),
			Connected: time.Now(), // 需要实际记录连接时间
			LastSeen:  time.Now(),
		})
	}

	return infos
}

// ListServers 列出所有服务器
func (m *Manager) ListServers() []ServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var infos []ServerInfo
	for host, _ := range m.servers {
		infos = append(infos, ServerInfo{
			ID:        host,
			Host:      host,
			StartedAt: time.Now(), // 需要实际记录启动时间
		})
	}

	return infos
}

// Close 关闭所有资源
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error

	// 关闭所有Agent
	for host, agent := range m.agents {
		if err := agent.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close agent %s: %w", host, err))
		}
	}

	// 关闭所有客户端
	for host, client := range m.clients {
		if err := client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close client %s: %w", host, err))
		}
	}

	// 停止所有服务器
	for host, server := range m.servers {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := server.Stop(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop server %s: %w", host, err))
		}
		cancel()
	}

	// 清空映射
	m.agents = make(map[string]Agent)
	m.clients = make(map[string]Client)
	m.servers = make(map[string]Server)

	if len(errors) > 0 {
		return fmt.Errorf("multiple errors occurred: %v", errors)
	}

	return nil
}

// 工具函数

// DefaultConfig 创建默认配置
func DefaultConfig(host string) AgentConfig {
	return AgentConfig{
		Host:        host,
		Port:        22,
		Timeout:     30 * time.Second,
		ControlPort: 8081,
		DataPort:    8080,
		BindAddress: "127.0.0.1",
		BinaryPath:  "",
		WorkDir:     "~/.devssh-agent",
		LogFile:     "agent.log",
	}
}

// ConfigFromSSH 从SSH配置创建Agent配置
func ConfigFromSSH(sshConfig *ssh.Config) AgentConfig {
	return AgentConfig{
		Host:        sshConfig.Host,
		Port:        parsePort(sshConfig.Port),
		Timeout:     sshConfig.Timeout,
		ControlPort: 8081,
		DataPort:    8080,
		BindAddress: "127.0.0.1",
		WorkDir:     "~/.devssh-agent",
	}
}

// 解析端口字符串
func parsePort(portStr string) int {
	if portStr == "" {
		return 22
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		return 22
	}

	return port
}

// 全局管理器实例
var globalManager *Manager
var managerOnce sync.Once

// GetManager 获取全局管理器
func GetManager() *Manager {
	managerOnce.Do(func() {
		globalManager = NewManager(NewDefaultFactory())
	})
	return globalManager
}

// CreateAgentWithSSH 使用SSH创建Agent
func CreateAgentWithSSH(sshClient *ssh.Client, config AgentConfig) (Agent, error) {
	// 创建部署器
	deployer := NewDeployer(sshClient)

	// 检查是否已部署
	deployed, err := deployer.CheckDeployed(context.Background())
	if err != nil {
		return nil, err
	}

	if !deployed {
		// 部署Agent
		if err := deployer.Deploy(context.Background(), config); err != nil {
			return nil, err
		}
	}

	// 创建客户端连接
	factory := NewDefaultFactory()
	return factory.CreateAgentFromSSH(sshClient, config)
}
