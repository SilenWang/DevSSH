package tunnel

import (
	"fmt"
	"net"
)

const (
	MaxPortRetries = 10    // 最大重试次数
	SystemPortMin  = 1024  // 系统端口最小值（避免使用<1024的端口）
	MaxPort        = 65535 // 最大端口号
)

// IsPortAvailable 检查端口是否可用
func IsPortAvailable(port int) bool {
	if port < 1 || port > MaxPort {
		return false
	}

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// FindAvailablePort 寻找可用端口，记录重试过程
func FindAvailablePort(startPort int, logFunc func(string)) (int, error) {
	// 确保起始端口不小于系统端口最小值
	if startPort < SystemPortMin {
		startPort = SystemPortMin
		logFunc(fmt.Sprintf("Starting port adjusted to %d (avoid system ports <1024)", startPort))
	}

	currentPort := startPort
	attempts := 0

	for attempts < MaxPortRetries {
		if currentPort > MaxPort {
			return 0, fmt.Errorf("port %d exceeds maximum port %d", currentPort, MaxPort)
		}

		if IsPortAvailable(currentPort) {
			if attempts > 0 {
				logFunc(fmt.Sprintf("Found available port %d after %d attempts", currentPort, attempts))
			}
			return currentPort, nil
		}

		// 记录重试过程
		if attempts == 0 {
			logFunc(fmt.Sprintf("Port %d is occupied, trying next ports...", startPort))
		}
		logFunc(fmt.Sprintf("  Port %d is occupied, trying port %d", currentPort, currentPort+1))

		currentPort++
		attempts++
	}

	return 0, fmt.Errorf("no available port found after %d attempts (starting from %d)", MaxPortRetries, startPort)
}
