package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"devssh/pkg/ssh"
)

// HTTPClient HTTP客户端实现
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	token      string
	agentID    string
	connected  bool
	mu         sync.RWMutex
	config     AgentConfig
	events     chan Event
	subscribed bool
}

// NewHTTPClient 创建新的HTTP客户端
func NewHTTPClient(config AgentConfig) (*HTTPClient, error) {
	if config.ControlPort == 0 {
		config.ControlPort = 8081
	}
	if config.BindAddress == "" {
		config.BindAddress = "127.0.0.1"
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	baseURL := fmt.Sprintf("http://%s:%d", config.BindAddress, config.ControlPort)

	return &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		token:     config.Token,
		config:    config,
		events:    make(chan Event, 100),
		connected: false,
	}, nil
}

// Connect 连接到Agent服务器
func (c *HTTPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 测试连接
	if err := c.ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to agent: %w", err)
	}

	// 获取Agent信息
	info, err := c.getAgentInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get agent info: %w", err)
	}

	c.agentID = info.ID
	c.connected = true

	// 启动事件监听
	go c.eventListener(ctx)

	return nil
}

// ID 返回Agent ID
func (c *HTTPClient) ID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.agentID
}

// Version 返回Agent版本
func (c *HTTPClient) Version() string {
	// 从服务器获取版本信息
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := c.getAgentInfo(ctx)
	if err != nil {
		return "unknown"
	}
	return info.Version
}

// Status 返回Agent状态
func (c *HTTPClient) Status() AgentStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return StatusStopped
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := c.getAgentInfo(ctx)
	if err != nil {
		return StatusError
	}
	return info.Status
}

// Start 启动Agent（客户端不支持）
func (c *HTTPClient) Start(ctx context.Context) error {
	return fmt.Errorf("client cannot start agent remotely")
}

// Stop 停止Agent（客户端不支持）
func (c *HTTPClient) Stop(ctx context.Context) error {
	return fmt.Errorf("client cannot stop agent remotely")
}

// Restart 重启Agent（客户端不支持）
func (c *HTTPClient) Restart(ctx context.Context) error {
	return fmt.Errorf("client cannot restart agent remotely")
}

// Update 更新Agent（客户端不支持）
func (c *HTTPClient) Update(ctx context.Context, version string) error {
	return fmt.Errorf("client cannot update agent remotely")
}

// InstallIDE 安装IDE
func (c *HTTPClient) InstallIDE(ctx context.Context, config IDEConfig) error {
	req := struct {
		Config IDEConfig `json:"config"`
	}{
		Config: config,
	}

	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPost, APIIDEInstall, req, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to install IDE: %s", resp.Message)
	}

	return nil
}

// StartIDE 启动IDE
func (c *HTTPClient) StartIDE(ctx context.Context, ideType IDEType, port int) error {
	req := struct {
		Type IDEType `json:"type"`
		Port int     `json:"port"`
	}{
		Type: ideType,
		Port: port,
	}

	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPost, APIIDEStart, req, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to start IDE: %s", resp.Message)
	}

	return nil
}

// StopIDE 停止IDE
func (c *HTTPClient) StopIDE(ctx context.Context, ideType IDEType) error {
	req := struct {
		Type IDEType `json:"type"`
	}{
		Type: ideType,
	}

	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPost, APIIDEStop, req, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to stop IDE: %s", resp.Message)
	}

	return nil
}

// RestartIDE 重启IDE
func (c *HTTPClient) RestartIDE(ctx context.Context, ideType IDEType) error {
	req := struct {
		Type IDEType `json:"type"`
	}{
		Type: ideType,
	}

	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPost, APIIDERestart, req, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to restart IDE: %s", resp.Message)
	}

	return nil
}

// GetIDEStatus 获取IDE状态
func (c *HTTPClient) GetIDEStatus(ctx context.Context, ideType IDEType) (IDEStatus, error) {
	path := strings.Replace(APIIDEStatus, "{type}", string(ideType), 1)

	var status IDEStatus
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &status); err != nil {
		return IDEStatus{}, err
	}

	return status, nil
}

// ListIDEs 列出所有IDE
func (c *HTTPClient) ListIDEs(ctx context.Context) ([]IDEStatus, error) {
	var resp struct {
		IDEs []IDEStatus `json:"ides"`
	}

	if err := c.doRequest(ctx, http.MethodGet, APIIDEList, nil, &resp); err != nil {
		return nil, err
	}

	return resp.IDEs, nil
}

// ExecuteCommand 执行命令
func (c *HTTPClient) ExecuteCommand(ctx context.Context, req CommandRequest) (CommandResponse, error) {
	var resp CommandResponse
	if err := c.doRequest(ctx, http.MethodPost, APICommandExecute, req, &resp); err != nil {
		return CommandResponse{}, err
	}

	return resp, nil
}

// ExecuteCommandStream 执行命令并获取流式输出
func (c *HTTPClient) ExecuteCommandStream(ctx context.Context, req CommandRequest) (io.ReadCloser, error) {
	// 设置流式输出
	req.Stream = true

	httpReq, err := c.createRequest(ctx, http.MethodPost, APICommandExecute, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("request failed with status: %s", resp.Status)
	}

	return resp.Body, nil
}

// CancelCommand 取消命令
func (c *HTTPClient) CancelCommand(ctx context.Context, commandID string) error {
	path := strings.Replace(APICommandCancel, "{id}", commandID, 1)

	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPost, path, nil, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to cancel command: %s", resp.Message)
	}

	return nil
}

// GetCommandStatus 获取命令状态
func (c *HTTPClient) GetCommandStatus(ctx context.Context, commandID string) (CommandStatus, error) {
	path := strings.Replace(APICommandStatus, "{id}", commandID, 1)

	var status CommandStatus
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &status); err != nil {
		return CommandStatus{}, err
	}

	return status, nil
}

// UploadFile 上传文件
func (c *HTTPClient) UploadFile(ctx context.Context, req FileTransferRequest) (FileTransferResponse, error) {
	var resp FileTransferResponse
	if err := c.doRequest(ctx, http.MethodPost, APIFileUpload, req, &resp); err != nil {
		return FileTransferResponse{}, err
	}

	return resp, nil
}

// DownloadFile 下载文件
func (c *HTTPClient) DownloadFile(ctx context.Context, path string) ([]byte, error) {
	req := struct {
		Path string `json:"path"`
	}{
		Path: path,
	}

	httpReq, err := c.createRequest(ctx, http.MethodPost, APIFileDownload, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// ListFiles 列出文件
func (c *HTTPClient) ListFiles(ctx context.Context, path string) ([]FileInfo, error) {
	req := struct {
		Path string `json:"path"`
	}{
		Path: path,
	}

	var resp struct {
		Files []FileInfo `json:"files"`
	}

	if err := c.doRequest(ctx, http.MethodPost, APIFileList, req, &resp); err != nil {
		return nil, err
	}

	return resp.Files, nil
}

// DeleteFile 删除文件
func (c *HTTPClient) DeleteFile(ctx context.Context, path string) error {
	req := struct {
		Path string `json:"path"`
	}{
		Path: path,
	}

	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPost, APIFileDelete, req, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to delete file: %s", resp.Message)
	}

	return nil
}

// AddPortForward 添加端口转发
func (c *HTTPClient) AddPortForward(ctx context.Context, config PortForwardConfig) error {
	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPost, APIPortAdd, config, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to add port forward: %s", resp.Message)
	}

	return nil
}

// RemovePortForward 移除端口转发
func (c *HTTPClient) RemovePortForward(ctx context.Context, name string) error {
	req := struct {
		Name string `json:"name"`
	}{
		Name: name,
	}

	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPost, APIPortRemove, req, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to remove port forward: %s", resp.Message)
	}

	return nil
}

// ListPortForwards 列出端口转发
func (c *HTTPClient) ListPortForwards(ctx context.Context) ([]PortForwardConfig, error) {
	var resp struct {
		Ports []PortForwardConfig `json:"ports"`
	}

	if err := c.doRequest(ctx, http.MethodGet, APIPortList, nil, &resp); err != nil {
		return nil, err
	}

	return resp.Ports, nil
}

// SubscribeEvents 订阅事件
func (c *HTTPClient) SubscribeEvents(ctx context.Context) (<-chan Event, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.subscribed {
		return c.events, nil
	}

	// 启动事件监听
	c.subscribed = true
	return c.events, nil
}

// UnsubscribeEvents 取消订阅事件
func (c *HTTPClient) UnsubscribeEvents(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.subscribed = false
	close(c.events)
	c.events = make(chan Event, 100)
	return nil
}

// SendHeartbeat 发送心跳
func (c *HTTPClient) SendHeartbeat(ctx context.Context) (HeartbeatResponse, error) {
	req := HeartbeatRequest{
		AgentID: c.agentID,
		Status:  c.Status(),
		Time:    time.Now(),
	}

	var resp HeartbeatResponse
	if err := c.doRequest(ctx, http.MethodPost, APIHeartbeat, req, &resp); err != nil {
		return HeartbeatResponse{}, err
	}

	return resp, nil
}

// GetConfig 获取配置
func (c *HTTPClient) GetConfig(ctx context.Context) (AgentConfig, error) {
	var config AgentConfig
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/config", nil, &config); err != nil {
		return AgentConfig{}, err
	}

	return config, nil
}

// UpdateConfig 更新配置
func (c *HTTPClient) UpdateConfig(ctx context.Context, config AgentConfig) error {
	var resp SuccessResponse
	if err := c.doRequest(ctx, http.MethodPut, "/api/v1/config", config, &resp); err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to update config: %s", resp.Message)
	}

	return nil
}

// Close 关闭客户端
func (c *HTTPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	c.subscribed = false
	close(c.events)
	c.httpClient.CloseIdleConnections()
	return nil
}

// IsConnected 检查是否已连接
func (c *HTTPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Disconnect 断开连接
func (c *HTTPClient) Disconnect() error {
	return c.Close()
}

// ServerInfo 返回服务器信息
func (c *HTTPClient) ServerInfo() ServerInfo {
	return ServerInfo{
		ID:        c.agentID,
		Host:      c.config.BindAddress,
		Port:      c.config.ControlPort,
		StartedAt: time.Now(),
	}
}

// 私有方法

func (c *HTTPClient) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status: %s", resp.Status)
	}

	return nil
}

func (c *HTTPClient) getAgentInfo(ctx context.Context) (AgentInfo, error) {
	var info AgentInfo
	if err := c.doRequest(ctx, http.MethodGet, APIAgentStatus, nil, &info); err != nil {
		return AgentInfo{}, err
	}
	return info, nil
}

func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	req, err := c.createRequest(ctx, method, path, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return fmt.Errorf("request failed: %s (code: %d)", errResp.Message, errResp.Code)
		}
		return fmt.Errorf("request failed with status: %s", resp.Status)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

func (c *HTTPClient) createRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	fullURL := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = strings.NewReader(string(jsonData))
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return req, nil
}

func (c *HTTPClient) eventListener(ctx context.Context) {
	for c.subscribed {
		select {
		case <-ctx.Done():
			return
		default:
			// 监听事件流
			if err := c.listenEvents(ctx); err != nil {
				// 重试延迟
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (c *HTTPClient) listenEvents(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+APIEvents, nil)
	if err != nil {
		return err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("event stream failed with status: %s", resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			var event Event
			if err := decoder.Decode(&event); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}

			// 发送事件到通道
			select {
			case c.events <- event:
			default:
				// 通道满，丢弃事件
			}
		}
	}
}

// SSHClient SSH客户端包装器，将SSH命令转换为Agent调用
type SSHClient struct {
	sshClient *ssh.Client
	agent     Agent
	config    AgentConfig
}

// NewSSHClient 创建SSH客户端包装器
func NewSSHClient(sshClient *ssh.Client, config AgentConfig) (*SSHClient, error) {
	return &SSHClient{
		sshClient: sshClient,
		config:    config,
	}, nil
}

// Connect 连接到远程Agent或部署Agent
func (sc *SSHClient) Connect(ctx context.Context) error {
	// 首先尝试连接到现有的Agent
	agentConfig := sc.config
	agentConfig.BindAddress = "127.0.0.1"

	agent, err := NewHTTPClient(agentConfig)
	if err != nil {
		return err
	}

	if err := agent.Connect(ctx); err == nil {
		sc.agent = agent
		return nil
	}

	// 如果连接失败，部署新的Agent
	deployer := NewDeployer(sc.sshClient)
	if err := deployer.Deploy(ctx, sc.config); err != nil {
		return fmt.Errorf("failed to deploy agent: %w", err)
	}

	// 等待Agent启动
	time.Sleep(2 * time.Second)

	// 再次尝试连接
	if err := agent.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to deployed agent: %w", err)
	}

	sc.agent = agent
	return nil
}

// 实现Agent接口的方法（委托给内部的agent）
func (sc *SSHClient) ID() string {
	if sc.agent == nil {
		return ""
	}
	return sc.agent.ID()
}

func (sc *SSHClient) Version() string {
	if sc.agent == nil {
		return ""
	}
	return sc.agent.Version()
}

func (sc *SSHClient) Status() AgentStatus {
	if sc.agent == nil {
		return StatusStopped
	}
	return sc.agent.Status()
}

// 其他方法类似，委托给内部的agent
// 为了简洁，这里省略了其他方法的实现
// 实际实现中需要为每个方法添加相应的委托调用

func (sc *SSHClient) Start(ctx context.Context) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.Start(ctx)
}

func (sc *SSHClient) Stop(ctx context.Context) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.Stop(ctx)
}

func (sc *SSHClient) Restart(ctx context.Context) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.Restart(ctx)
}

func (sc *SSHClient) Update(ctx context.Context, version string) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.Update(ctx, version)
}

func (sc *SSHClient) InstallIDE(ctx context.Context, config IDEConfig) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.InstallIDE(ctx, config)
}

func (sc *SSHClient) StartIDE(ctx context.Context, ideType IDEType, port int) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.StartIDE(ctx, ideType, port)
}

func (sc *SSHClient) StopIDE(ctx context.Context, ideType IDEType) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.StopIDE(ctx, ideType)
}

func (sc *SSHClient) RestartIDE(ctx context.Context, ideType IDEType) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.RestartIDE(ctx, ideType)
}

func (sc *SSHClient) GetIDEStatus(ctx context.Context, ideType IDEType) (IDEStatus, error) {
	if sc.agent == nil {
		return IDEStatus{}, fmt.Errorf("agent not initialized")
	}
	return sc.agent.GetIDEStatus(ctx, ideType)
}

func (sc *SSHClient) ListIDEs(ctx context.Context) ([]IDEStatus, error) {
	if sc.agent == nil {
		return nil, fmt.Errorf("agent not initialized")
	}
	return sc.agent.ListIDEs(ctx)
}

func (sc *SSHClient) ExecuteCommand(ctx context.Context, req CommandRequest) (CommandResponse, error) {
	if sc.agent == nil {
		return CommandResponse{}, fmt.Errorf("agent not initialized")
	}
	return sc.agent.ExecuteCommand(ctx, req)
}

func (sc *SSHClient) ExecuteCommandStream(ctx context.Context, req CommandRequest) (io.ReadCloser, error) {
	if sc.agent == nil {
		return nil, fmt.Errorf("agent not initialized")
	}
	return sc.agent.ExecuteCommandStream(ctx, req)
}

func (sc *SSHClient) CancelCommand(ctx context.Context, commandID string) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.CancelCommand(ctx, commandID)
}

func (sc *SSHClient) GetCommandStatus(ctx context.Context, commandID string) (CommandStatus, error) {
	if sc.agent == nil {
		return CommandStatus{}, fmt.Errorf("agent not initialized")
	}
	return sc.agent.GetCommandStatus(ctx, commandID)
}

func (sc *SSHClient) UploadFile(ctx context.Context, req FileTransferRequest) (FileTransferResponse, error) {
	if sc.agent == nil {
		return FileTransferResponse{}, fmt.Errorf("agent not initialized")
	}
	return sc.agent.UploadFile(ctx, req)
}

func (sc *SSHClient) DownloadFile(ctx context.Context, path string) ([]byte, error) {
	if sc.agent == nil {
		return nil, fmt.Errorf("agent not initialized")
	}
	return sc.agent.DownloadFile(ctx, path)
}

func (sc *SSHClient) ListFiles(ctx context.Context, path string) ([]FileInfo, error) {
	if sc.agent == nil {
		return nil, fmt.Errorf("agent not initialized")
	}
	return sc.agent.ListFiles(ctx, path)
}

func (sc *SSHClient) DeleteFile(ctx context.Context, path string) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.DeleteFile(ctx, path)
}

func (sc *SSHClient) AddPortForward(ctx context.Context, config PortForwardConfig) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.AddPortForward(ctx, config)
}

func (sc *SSHClient) RemovePortForward(ctx context.Context, name string) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.RemovePortForward(ctx, name)
}

func (sc *SSHClient) ListPortForwards(ctx context.Context) ([]PortForwardConfig, error) {
	if sc.agent == nil {
		return nil, fmt.Errorf("agent not initialized")
	}
	return sc.agent.ListPortForwards(ctx)
}

func (sc *SSHClient) SubscribeEvents(ctx context.Context) (<-chan Event, error) {
	if sc.agent == nil {
		return nil, fmt.Errorf("agent not initialized")
	}
	return sc.agent.SubscribeEvents(ctx)
}

func (sc *SSHClient) UnsubscribeEvents(ctx context.Context) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.UnsubscribeEvents(ctx)
}

func (sc *SSHClient) SendHeartbeat(ctx context.Context) (HeartbeatResponse, error) {
	if sc.agent == nil {
		return HeartbeatResponse{}, fmt.Errorf("agent not initialized")
	}
	return sc.agent.SendHeartbeat(ctx)
}

func (sc *SSHClient) GetConfig(ctx context.Context) (AgentConfig, error) {
	if sc.agent == nil {
		return AgentConfig{}, fmt.Errorf("agent not initialized")
	}
	return sc.agent.GetConfig(ctx)
}

func (sc *SSHClient) UpdateConfig(ctx context.Context, config AgentConfig) error {
	if sc.agent == nil {
		return fmt.Errorf("agent not initialized")
	}
	return sc.agent.UpdateConfig(ctx, config)
}

func (sc *SSHClient) Close() error {
	if sc.agent != nil {
		return sc.agent.Close()
	}
	return nil
}

func (sc *SSHClient) IsConnected() bool {
	return sc.agent != nil && sc.agent.Status() == StatusRunning
}

func (sc *SSHClient) Disconnect() error {
	return sc.Close()
}

func (sc *SSHClient) ServerInfo() ServerInfo {
	if sc.agent != nil {
		// 尝试类型断言获取ServerInfo
		if client, ok := sc.agent.(Client); ok {
			return client.ServerInfo()
		}
	}
	return ServerInfo{}
}
