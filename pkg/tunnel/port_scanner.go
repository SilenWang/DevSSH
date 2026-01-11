package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/sylens/project/DevSSH/pkg/ssh"
)

type PortInfo struct {
	Port     int
	Protocol string
	Service  string
	Process  string
}

type PortScanner struct {
	sshClient *ssh.Client
}

func NewPortScanner(sshClient *ssh.Client) *PortScanner {
	return &PortScanner{
		sshClient: sshClient,
	}
}

func (s *PortScanner) ScanCommonPorts() ([]PortInfo, error) {
	commonPorts := []int{
		// Web servers
		80,   // HTTP
		443,  // HTTPS
		3000, // Node.js, React
		8080, // Alternative HTTP
		8000, // Django, Flask
		8888, // Jupyter

		// Development servers
		3001, // React (alt)
		4200, // Angular
		5000, // Flask
		5173, // Vite
		6000, // X11

		// Database
		3306,  // MySQL
		5432,  // PostgreSQL
		27017, // MongoDB
		6379,  // Redis

		// Message brokers
		5672, // RabbitMQ
		9092, // Kafka

		// Other services
		22, // SSH
		21, // FTP
		25, // SMTP
		53, // DNS
	}

	var openPorts []PortInfo

	for _, port := range commonPorts {
		if s.isPortOpen(port) {
			info := PortInfo{
				Port:     port,
				Protocol: "tcp",
				Service:  s.guessService(port),
			}
			openPorts = append(openPorts, info)
		}
	}

	return openPorts, nil
}

func (s *PortScanner) ScanPortRange(start, end int) ([]PortInfo, error) {
	var openPorts []PortInfo

	for port := start; port <= end; port++ {
		if s.isPortOpen(port) {
			info := PortInfo{
				Port:     port,
				Protocol: "tcp",
				Service:  s.guessService(port),
			}
			openPorts = append(openPorts, info)
		}
	}

	return openPorts, nil
}

func (s *PortScanner) GetListeningPorts() ([]PortInfo, error) {
	// 使用 netstat 或 ss 命令获取监听端口
	commands := []string{
		"ss -tuln 2>/dev/null",
		"netstat -tuln 2>/dev/null",
	}

	var output string
	var err error

	for _, cmd := range commands {
		output, err = s.sshClient.RunCommand(cmd)
		if err == nil && output != "" {
			break
		}
	}

	if err != nil || output == "" {
		return nil, fmt.Errorf("failed to get listening ports: %w", err)
	}

	return s.parseNetstatOutput(output), nil
}

func (s *PortScanner) isPortOpen(port int) bool {
	// 使用 nc 或 telnet 检查端口
	checkCommands := []string{
		fmt.Sprintf("timeout 1 bash -c '</dev/tcp/localhost/%d' 2>/dev/null && echo open", port),
		fmt.Sprintf("nc -z localhost %d 2>/dev/null && echo open", port),
		fmt.Sprintf("timeout 1 telnet localhost %d 2>/dev/null | grep -q Connected && echo open", port),
	}

	for _, cmd := range checkCommands {
		output, err := s.sshClient.RunCommand(cmd)
		if err == nil && strings.Contains(output, "open") {
			return true
		}
	}

	return false
}

func (s *PortScanner) parseNetstatOutput(output string) []PortInfo {
	var ports []PortInfo

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Proto") || strings.HasPrefix(line, "State") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// 解析地址字段 (格式: 0.0.0.0:8080 或 :::8080)
		localAddr := fields[3]
		portStr := ""

		if idx := strings.LastIndex(localAddr, ":"); idx != -1 {
			portStr = localAddr[idx+1:]
		}

		if portStr == "" || portStr == "*" {
			continue
		}

		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}

		// 获取协议
		protocol := "tcp"
		if len(fields) > 0 {
			proto := strings.ToLower(fields[0])
			if strings.Contains(proto, "udp") {
				protocol = "udp"
			}
		}

		// 获取进程信息
		process := ""
		if len(fields) > 6 {
			process = fields[6]
		}

		info := PortInfo{
			Port:     port,
			Protocol: protocol,
			Service:  s.guessService(port),
			Process:  process,
		}

		ports = append(ports, info)
	}

	return ports
}

func (s *PortScanner) guessService(port int) string {
	serviceMap := map[int]string{
		22:    "SSH",
		80:    "HTTP",
		443:   "HTTPS",
		3000:  "Node.js/React",
		3001:  "React (alt)",
		4200:  "Angular",
		5000:  "Flask",
		5173:  "Vite",
		6000:  "X11",
		8080:  "HTTP Proxy/Web IDE",
		8000:  "Django/Flask",
		8888:  "Jupyter",
		3306:  "MySQL",
		5432:  "PostgreSQL",
		27017: "MongoDB",
		6379:  "Redis",
		5672:  "RabbitMQ",
		9092:  "Kafka",
		21:    "FTP",
		25:    "SMTP",
		53:    "DNS",
	}

	if service, ok := serviceMap[port]; ok {
		return service
	}

	return "Unknown"
}

func (s *PortScanner) DetectWebServices() ([]PortInfo, error) {
	allPorts, err := s.GetListeningPorts()
	if err != nil {
		return nil, err
	}

	var webPorts []PortInfo
	webPortNumbers := map[int]bool{
		80:   true,
		443:  true,
		3000: true,
		3001: true,
		4200: true,
		5000: true,
		5173: true,
		8080: true,
		8000: true,
		8888: true,
	}

	for _, port := range allPorts {
		if webPortNumbers[port.Port] {
			webPorts = append(webPorts, port)
		}
	}

	return webPorts, nil
}

func (s *PortScanner) CheckServiceHealth(port int) (bool, error) {
	// 尝试连接并发送简单的 HTTP 请求
	checkScript := fmt.Sprintf(`
timeout 2 bash -c '
if command -v curl &> /dev/null; then
	curl -s -f http://localhost:%d > /dev/null && echo "healthy"
elif command -v wget &> /dev/null; then
	wget -q -O /dev/null http://localhost:%d && echo "healthy"
else
	# 简单 TCP 连接检查
	timeout 1 bash -c "</dev/tcp/localhost/%d" 2>/dev/null && echo "healthy"
fi
'`, port, port, port)

	output, err := s.sshClient.RunCommand(checkScript)
	if err != nil {
		return false, nil
	}

	return strings.Contains(output, "healthy"), nil
}
