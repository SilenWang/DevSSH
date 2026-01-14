package agent

import (
	"context"
	"io"
	"time"
)

// Agent接口定义
type Agent interface {
	// 基本信息
	ID() string
	Version() string
	Status() AgentStatus

	// 生命周期管理
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	Update(ctx context.Context, version string) error

	// IDE管理
	InstallIDE(ctx context.Context, config IDEConfig) error
	StartIDE(ctx context.Context, ideType IDEType, port int) error
	StopIDE(ctx context.Context, ideType IDEType) error
	RestartIDE(ctx context.Context, ideType IDEType) error
	GetIDEStatus(ctx context.Context, ideType IDEType) (IDEStatus, error)
	ListIDEs(ctx context.Context) ([]IDEStatus, error)

	// 命令执行
	ExecuteCommand(ctx context.Context, req CommandRequest) (CommandResponse, error)
	ExecuteCommandStream(ctx context.Context, req CommandRequest) (io.ReadCloser, error)
	CancelCommand(ctx context.Context, commandID string) error
	GetCommandStatus(ctx context.Context, commandID string) (CommandStatus, error)

	// 文件操作
	UploadFile(ctx context.Context, req FileTransferRequest) (FileTransferResponse, error)
	DownloadFile(ctx context.Context, path string) ([]byte, error)
	ListFiles(ctx context.Context, path string) ([]FileInfo, error)
	DeleteFile(ctx context.Context, path string) error

	// 端口转发
	AddPortForward(ctx context.Context, config PortForwardConfig) error
	RemovePortForward(ctx context.Context, name string) error
	ListPortForwards(ctx context.Context) ([]PortForwardConfig, error)

	// 事件订阅
	SubscribeEvents(ctx context.Context) (<-chan Event, error)
	UnsubscribeEvents(ctx context.Context) error

	// 心跳
	SendHeartbeat(ctx context.Context) (HeartbeatResponse, error)

	// 配置管理
	GetConfig(ctx context.Context) (AgentConfig, error)
	UpdateConfig(ctx context.Context, config AgentConfig) error

	// 清理
	Close() error
}

// Agent客户端接口
type Client interface {
	Agent

	// 连接管理
	Connect(ctx context.Context) error
	IsConnected() bool
	Disconnect() error

	// 服务器信息
	ServerInfo() ServerInfo
}

// Agent服务端接口
type Server interface {
	// 服务端管理
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Serve(ctx context.Context) error

	// 客户端管理
	AddClient(client Client) error
	RemoveClient(clientID string) error
	ListClients() []ClientInfo

	// 统计信息
	GetStats() ServerStats
}

// 命令状态
type CommandStatus struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"`
	ExitCode  int           `json:"exit_code,omitempty"`
	StartTime time.Time     `json:"start_time,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
}

// 文件信息
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    int       `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

// 服务器信息
type ServerInfo struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
}

// 客户端信息
type ClientInfo struct {
	ID        string      `json:"id"`
	AgentID   string      `json:"agent_id"`
	Status    AgentStatus `json:"status"`
	Connected time.Time   `json:"connected"`
	LastSeen  time.Time   `json:"last_seen"`
}

// 服务器统计
type ServerStats struct {
	Uptime           time.Duration `json:"uptime"`
	ClientsConnected int           `json:"clients_connected"`
	ClientsTotal     int           `json:"clients_total"`
	CommandsExecuted int           `json:"commands_executed"`
	FilesTransferred int           `json:"files_transferred"`
	PortsForwarded   int           `json:"ports_forwarded"`
	IDEsRunning      int           `json:"ides_running"`
	MemoryUsage      int64         `json:"memory_usage"`
	CPUUsage         float64       `json:"cpu_usage"`
}

// Agent工厂接口
type Factory interface {
	// 创建Agent
	CreateAgent(config AgentConfig) (Agent, error)

	// 创建客户端
	CreateClient(config AgentConfig) (Client, error)

	// 创建服务端
	CreateServer(config AgentConfig) (Server, error)

	// 从SSH客户端创建Agent
	CreateAgentFromSSH(sshClient interface{}, config AgentConfig) (Agent, error)
}

// 事件处理器接口
type EventHandler interface {
	HandleEvent(event Event) error
}

// 日志接口
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Fatal(msg string, args ...interface{})

	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
}

// 配置存储接口
type ConfigStore interface {
	// Agent配置
	SaveAgentConfig(agentID string, config AgentConfig) error
	LoadAgentConfig(agentID string) (AgentConfig, error)
	DeleteAgentConfig(agentID string) error
	ListAgentConfigs() ([]string, error)

	// IDE配置
	SaveIDEConfig(agentID string, ideType IDEType, config IDEConfig) error
	LoadIDEConfig(agentID string, ideType IDEType) (IDEConfig, error)
	DeleteIDEConfig(agentID string, ideType IDEType) error
	ListIDEs(agentID string) ([]IDEType, error)

	// 状态存储
	SaveAgentStatus(agentID string, status AgentInfo) error
	LoadAgentStatus(agentID string) (AgentInfo, error)

	// 清理
	Close() error
}
