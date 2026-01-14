package agent

import (
	"time"
)

// Agent状态
type AgentStatus string

const (
	StatusStopped    AgentStatus = "stopped"
	StatusStarting   AgentStatus = "starting"
	StatusRunning    AgentStatus = "running"
	StatusStopping   AgentStatus = "stopping"
	StatusError      AgentStatus = "error"
	StatusInstalling AgentStatus = "installing"
)

// IDE类型
type IDEType string

const (
	IDETypeVSCode     IDEType = "vscode"
	IDETypeCodeServer IDEType = "code-server"
	IDETypeJupyter    IDEType = "jupyter"
	IDETypeTheia      IDEType = "theia"
)

// Agent配置
type AgentConfig struct {
	// 基础配置
	Host    string        `json:"host"`
	Port    int           `json:"port"`
	Token   string        `json:"token,omitempty"`
	Timeout time.Duration `json:"timeout"`

	// 部署配置
	BinaryPath string `json:"binary_path,omitempty"`
	WorkDir    string `json:"work_dir,omitempty"`
	LogFile    string `json:"log_file,omitempty"`

	// 通信配置
	ControlPort int    `json:"control_port"`
	DataPort    int    `json:"data_port"`
	BindAddress string `json:"bind_address"`
}

// Agent信息
type AgentInfo struct {
	ID        string      `json:"id"`
	Status    AgentStatus `json:"status"`
	Version   string      `json:"version"`
	PID       int         `json:"pid,omitempty"`
	StartTime time.Time   `json:"start_time,omitempty"`
	Config    AgentConfig `json:"config"`
}

// IDE配置
type IDEConfig struct {
	Type    IDEType           `json:"type"`
	Version string            `json:"version,omitempty"`
	Port    int               `json:"port"`
	Options map[string]string `json:"options,omitempty"`
}

// IDE状态
type IDEStatus struct {
	Type    IDEType   `json:"type"`
	Status  string    `json:"status"`
	Port    int       `json:"port"`
	PID     int       `json:"pid,omitempty"`
	URL     string    `json:"url,omitempty"`
	Started time.Time `json:"started,omitempty"`
	Config  IDEConfig `json:"config"`
}

// 命令请求
type CommandRequest struct {
	ID      string            `json:"id"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Timeout time.Duration     `json:"timeout,omitempty"`
	Stream  bool              `json:"stream,omitempty"`
	WorkDir string            `json:"work_dir,omitempty"`
}

// 命令响应
type CommandResponse struct {
	ID        string    `json:"id"`
	ExitCode  int       `json:"exit_code"`
	Stdout    string    `json:"stdout,omitempty"`
	Stderr    string    `json:"stderr,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// 文件传输请求
type FileTransferRequest struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Content  []byte `json:"content,omitempty"`
	Mode     int    `json:"mode,omitempty"`
	Append   bool   `json:"append,omitempty"`
	Checksum string `json:"checksum,omitempty"`
}

// 文件传输响应
type FileTransferResponse struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum,omitempty"`
	Error    string `json:"error,omitempty"`
}

// 端口转发配置
type PortForwardConfig struct {
	Name       string `json:"name"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"`
	Protocol   string `json:"protocol,omitempty"`
	Enabled    bool   `json:"enabled"`
}

// 心跳请求
type HeartbeatRequest struct {
	AgentID string      `json:"agent_id"`
	Status  AgentStatus `json:"status"`
	Time    time.Time   `json:"time"`
}

// 心跳响应
type HeartbeatResponse struct {
	Time    time.Time `json:"time"`
	Command string    `json:"command,omitempty"`
}

// 事件类型
type EventType string

const (
	EventAgentStarted    EventType = "agent_started"
	EventAgentStopped    EventType = "agent_stopped"
	EventAgentError      EventType = "agent_error"
	EventIDEInstalled    EventType = "ide_installed"
	EventIDEStarted      EventType = "ide_started"
	EventIDEStopped      EventType = "ide_stopped"
	EventPortForwarded   EventType = "port_forwarded"
	EventCommandStarted  EventType = "command_started"
	EventCommandFinished EventType = "command_finished"
	EventFileTransferred EventType = "file_transferred"
)

// 事件
type Event struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
	AgentID   string      `json:"agent_id,omitempty"`
}

// API端点定义
const (
	// Agent管理
	APIAgentStatus  = "/api/v1/agent/status"
	APIAgentStart   = "/api/v1/agent/start"
	APIAgentStop    = "/api/v1/agent/stop"
	APIAgentRestart = "/api/v1/agent/restart"
	APIAgentUpdate  = "/api/v1/agent/update"

	// IDE管理
	APIIDEList    = "/api/v1/ide"
	APIIDEInstall = "/api/v1/ide/install"
	APIIDEStart   = "/api/v1/ide/start"
	APIIDEStop    = "/api/v1/ide/stop"
	APIIDERestart = "/api/v1/ide/restart"
	APIIDEStatus  = "/api/v1/ide/{type}/status"

	// 命令执行
	APICommandExecute = "/api/v1/command/execute"
	APICommandStatus  = "/api/v1/command/{id}/status"
	APICommandCancel  = "/api/v1/command/{id}/cancel"

	// 文件操作
	APIFileUpload   = "/api/v1/file/upload"
	APIFileDownload = "/api/v1/file/download"
	APIFileList     = "/api/v1/file/list"
	APIFileDelete   = "/api/v1/file/delete"

	// 端口转发
	APIPortList   = "/api/v1/port"
	APIPortAdd    = "/api/v1/port/add"
	APIPortRemove = "/api/v1/port/remove"

	// 事件流
	APIEvents = "/api/v1/events"

	// 心跳
	APIHeartbeat = "/api/v1/heartbeat"
)

// 错误响应
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// 成功响应
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}
