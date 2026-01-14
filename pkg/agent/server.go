package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// HTTPServer HTTP服务器实现
type HTTPServer struct {
	config      AgentConfig
	server      *http.Server
	listener    net.Listener
	agentInfo   AgentInfo
	clients     map[string]ClientInfo
	mu          sync.RWMutex
	startTime   time.Time
	eventChan   chan Event
	subscribers map[string]chan Event
	stopChan    chan struct{}
	running     bool
	logger      *logrus.Logger
}

// NewHTTPServer 创建新的HTTP服务器
func NewHTTPServer(config AgentConfig) (*HTTPServer, error) {
	if config.ControlPort == 0 {
		config.ControlPort = 8081
	}
	if config.BindAddress == "" {
		config.BindAddress = "127.0.0.1"
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	// 生成Agent ID
	agentID := generateAgentID()

	// 创建日志器
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	return &HTTPServer{
		config: config,
		agentInfo: AgentInfo{
			ID:        agentID,
			Status:    StatusStopped,
			Version:   "0.1.0",
			StartTime: time.Now(),
			Config:    config,
		},
		clients:     make(map[string]ClientInfo),
		eventChan:   make(chan Event, 100),
		subscribers: make(map[string]chan Event),
		stopChan:    make(chan struct{}),
		logger:      logger,
	}, nil
}

// Start 启动服务器
func (s *HTTPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	// 创建监听器
	addr := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.ControlPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = listener

	// 创建HTTP服务器
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  s.config.Timeout,
		WriteTimeout: s.config.Timeout,
		IdleTimeout:  60 * time.Second,
	}

	// 更新状态
	s.agentInfo.Status = StatusStarting
	s.agentInfo.StartTime = time.Now()
	s.startTime = time.Now()
	s.running = true

	// 启动事件处理器
	go s.eventHandler()

	// 启动HTTP服务器
	go func() {
		s.logger.Infof("Starting agent server on %s", addr)
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Errorf("Server error: %v", err)
			s.mu.Lock()
			s.running = false
			s.agentInfo.Status = StatusError
			s.mu.Unlock()
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 测试服务器是否运行
	if err := s.testServer(); err != nil {
		s.server.Close()
		s.running = false
		s.agentInfo.Status = StatusError
		return fmt.Errorf("server failed to start: %w", err)
	}

	s.agentInfo.Status = StatusRunning
	s.logger.Info("Agent server started successfully")

	return nil
}

// Stop 停止服务器
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping agent server...")
	s.agentInfo.Status = StatusStopping

	// 关闭事件通道
	close(s.stopChan)

	// 关闭所有订阅者
	for id, ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, id)
	}

	// 关闭HTTP服务器
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			s.logger.Errorf("Failed to shutdown server: %v", err)
			s.server.Close()
		}
	}

	s.running = false
	s.agentInfo.Status = StatusStopped
	s.logger.Info("Agent server stopped")

	return nil
}

// Serve 运行服务器（阻塞）
func (s *HTTPServer) Serve(ctx context.Context) error {
	if err := s.Start(ctx); err != nil {
		return err
	}

	// 等待停止信号
	<-ctx.Done()
	return s.Stop(context.Background())
}

// AddClient 添加客户端
func (s *HTTPServer) AddClient(client Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	clientInfo := ClientInfo{
		ID:        generateClientID(),
		AgentID:   s.agentInfo.ID,
		Status:    client.Status(),
		Connected: time.Now(),
		LastSeen:  time.Now(),
	}

	s.clients[clientInfo.ID] = clientInfo

	// 发送事件
	s.sendEvent(Event{
		Type:      "client_connected",
		Timestamp: time.Now(),
		Data:      clientInfo,
		AgentID:   s.agentInfo.ID,
	})

	return nil
}

// RemoveClient 移除客户端
func (s *HTTPServer) RemoveClient(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if client, exists := s.clients[clientID]; exists {
		delete(s.clients, clientID)

		// 发送事件
		s.sendEvent(Event{
			Type:      "client_disconnected",
			Timestamp: time.Now(),
			Data:      client,
			AgentID:   s.agentInfo.ID,
		})
	}

	return nil
}

// ListClients 列出所有客户端
func (s *HTTPServer) ListClients() []ClientInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clients := make([]ClientInfo, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}

	return clients
}

// GetStats 获取服务器统计信息
func (s *HTTPServer) GetStats() ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ServerStats{
		Uptime:           time.Since(s.startTime),
		ClientsConnected: len(s.clients),
		ClientsTotal:     len(s.clients),
		CommandsExecuted: 0, // 需要在实际命令执行时更新
		FilesTransferred: 0, // 需要在实际文件传输时更新
		PortsForwarded:   0, // 需要在实际端口转发时更新
		IDEsRunning:      0, // 需要在实际IDE运行时更新
		MemoryUsage:      0, // 需要实际获取内存使用
		CPUUsage:         0, // 需要实际获取CPU使用
	}
}

// 设置路由
func (s *HTTPServer) setupRoutes(mux *http.ServeMux) {
	// 健康检查
	mux.HandleFunc("/health", s.handleHealth)

	// Agent管理
	mux.HandleFunc(APIAgentStatus, s.handleAgentStatus)
	mux.HandleFunc(APIAgentStart, s.handleAgentStart)
	mux.HandleFunc(APIAgentStop, s.handleAgentStop)
	mux.HandleFunc(APIAgentRestart, s.handleAgentRestart)
	mux.HandleFunc(APIAgentUpdate, s.handleAgentUpdate)

	// IDE管理
	mux.HandleFunc(APIIDEList, s.handleIDEList)
	mux.HandleFunc(APIIDEInstall, s.handleIDEInstall)
	mux.HandleFunc(APIIDEStart, s.handleIDEStart)
	mux.HandleFunc(APIIDEStop, s.handleIDEStop)
	mux.HandleFunc(APIIDERestart, s.handleIDERestart)
	mux.HandleFunc("/api/v1/ide/", s.handleIDEStatus)

	// 命令执行
	mux.HandleFunc(APICommandExecute, s.handleCommandExecute)
	mux.HandleFunc("/api/v1/command/", s.handleCommandStatus)

	// 文件操作
	mux.HandleFunc(APIFileUpload, s.handleFileUpload)
	mux.HandleFunc(APIFileDownload, s.handleFileDownload)
	mux.HandleFunc(APIFileList, s.handleFileList)
	mux.HandleFunc(APIFileDelete, s.handleFileDelete)

	// 端口转发
	mux.HandleFunc(APIPortList, s.handlePortList)
	mux.HandleFunc(APIPortAdd, s.handlePortAdd)
	mux.HandleFunc(APIPortRemove, s.handlePortRemove)

	// 事件流
	mux.HandleFunc(APIEvents, s.handleEvents)

	// 心跳
	mux.HandleFunc(APIHeartbeat, s.handleHeartbeat)

	// 配置
	mux.HandleFunc("/api/v1/config", s.handleConfig)
}

// 处理器方法
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"version": s.agentInfo.Version,
		"uptime":  time.Since(s.startTime).String(),
	})
}

func (s *HTTPServer) handleAgentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	info := s.agentInfo
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *HTTPServer) handleAgentStart(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Agent already running", http.StatusBadRequest)
}

func (s *HTTPServer) handleAgentStop(w http.ResponseWriter, r *http.Request) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.Stop(ctx)
	}()

	s.respondSuccess(w, "Agent stopping")
}

func (s *HTTPServer) handleAgentRestart(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handleAgentUpdate(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handleIDEList(w http.ResponseWriter, r *http.Request) {
	// 返回空列表，实际实现需要从存储中获取
	s.respondSuccess(w, map[string]interface{}{
		"ides": []interface{}{},
	})
}

func (s *HTTPServer) handleIDEInstall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config IDEConfig `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 实际安装逻辑
	s.logger.Infof("Installing IDE: %s", req.Config.Type)

	// 模拟安装
	time.Sleep(2 * time.Second)

	s.respondSuccess(w, "IDE installed")
}

func (s *HTTPServer) handleIDEStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type IDEType `json:"type"`
		Port int     `json:"port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 实际启动逻辑
	s.logger.Infof("Starting IDE: %s on port %d", req.Type, req.Port)

	s.respondSuccess(w, "IDE started")
}

func (s *HTTPServer) handleIDEStop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type IDEType `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 实际停止逻辑
	s.logger.Infof("Stopping IDE: %s", req.Type)

	s.respondSuccess(w, "IDE stopped")
}

func (s *HTTPServer) handleIDERestart(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handleIDEStatus(w http.ResponseWriter, r *http.Request) {
	// 解析IDE类型
	ideType := r.URL.Path[len("/api/v1/ide/"):]
	if strings.Contains(ideType, "/") {
		ideType = ideType[:strings.Index(ideType, "/")]
	}

	// 返回IDE状态
	status := IDEStatus{
		Type:   IDEType(ideType),
		Status: "stopped",
		Port:   0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *HTTPServer) handleCommandExecute(w http.ResponseWriter, r *http.Request) {
	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 生成命令ID
	if req.ID == "" {
		req.ID = generateCommandID()
	}

	// 实际执行逻辑
	s.logger.Infof("Executing command: %s", req.Command)

	// 模拟执行
	resp := CommandResponse{
		ID:        req.ID,
		ExitCode:  0,
		Stdout:    "Command executed successfully",
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *HTTPServer) handleCommandStatus(w http.ResponseWriter, r *http.Request) {
	// 解析命令ID
	path := r.URL.Path[len("/api/v1/command/"):]
	if strings.Contains(path, "/") {
		path = path[:strings.Index(path, "/")]
	}

	// 返回命令状态
	status := CommandStatus{
		ID:     path,
		Status: "finished",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *HTTPServer) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handleFileList(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handlePortList(w http.ResponseWriter, r *http.Request) {
	// 返回空列表
	s.respondSuccess(w, map[string]interface{}{
		"ports": []interface{}{},
	})
}

func (s *HTTPServer) handlePortAdd(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handlePortRemove(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, "Not implemented", http.StatusNotImplemented)
}

func (s *HTTPServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	// 设置SSE头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 创建订阅者
	subscriberID := generateSubscriberID()
	eventChan := make(chan Event, 10)

	s.mu.Lock()
	s.subscribers[subscriberID] = eventChan
	s.mu.Unlock()

	// 确保清理
	defer func() {
		s.mu.Lock()
		delete(s.subscribers, subscriberID)
		close(eventChan)
		s.mu.Unlock()
	}()

	// 发送初始事件
	initEvent := Event{
		Type:      "connected",
		Timestamp: time.Now(),
		Data:      map[string]string{"subscriber_id": subscriberID},
		AgentID:   s.agentInfo.ID,
	}

	if err := s.sendSSEEvent(w, initEvent); err != nil {
		return
	}

	// 监听事件
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-eventChan:
			if err := s.sendSSEEvent(w, event); err != nil {
				return
			}
		case <-s.stopChan:
			return
		}
	}
}

func (s *HTTPServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 更新客户端最后活跃时间
	s.mu.Lock()
	if client, exists := s.clients[req.AgentID]; exists {
		client.LastSeen = time.Now()
		s.clients[req.AgentID] = client
	}
	s.mu.Unlock()

	resp := HeartbeatResponse{
		Time: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *HTTPServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		config := s.config
		s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)

	case http.MethodPut:
		var newConfig AgentConfig
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			s.respondError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		s.config = newConfig
		s.agentInfo.Config = newConfig
		s.mu.Unlock()

		s.respondSuccess(w, "Config updated")

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// 辅助方法
func (s *HTTPServer) respondSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := SuccessResponse{
		Success: true,
		Data:    data,
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *HTTPServer) respondError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	resp := ErrorResponse{
		Error:   http.StatusText(code),
		Code:    code,
		Message: message,
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *HTTPServer) sendSSEEvent(w http.ResponseWriter, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	if err != nil {
		return err
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}

func (s *HTTPServer) sendEvent(event Event) {
	select {
	case s.eventChan <- event:
	default:
		// 通道满，丢弃事件
		s.logger.Warn("Event channel full, dropping event")
	}
}

func (s *HTTPServer) eventHandler() {
	for {
		select {
		case event := <-s.eventChan:
			// 广播事件给所有订阅者
			s.mu.RLock()
			for _, ch := range s.subscribers {
				select {
				case ch <- event:
				default:
					// 订阅者通道满，跳过
				}
			}
			s.mu.RUnlock()

		case <-s.stopChan:
			return
		}
	}
}

func (s *HTTPServer) testServer() error {
	url := fmt.Sprintf("http://%s:%d/health", s.config.BindAddress, s.config.ControlPort)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %s", resp.Status)
	}

	return nil
}

// 工具函数
func generateAgentID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("agent-%s-%d", hostname, time.Now().Unix())
}

func generateClientID() string {
	return fmt.Sprintf("client-%d", time.Now().UnixNano())
}

func generateCommandID() string {
	return fmt.Sprintf("cmd-%d", time.Now().UnixNano())
}

func generateSubscriberID() string {
	return fmt.Sprintf("sub-%d", time.Now().UnixNano())
}

// 从路径解析IDE类型
func parseIDETypeFromPath(path string) string {
	// 移除路径前缀和后缀
	path = strings.TrimPrefix(path, "/api/v1/ide/")
	path = strings.TrimSuffix(path, "/status")
	return path
}
