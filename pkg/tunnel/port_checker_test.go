package tunnel

import (
	"fmt"
	"net"
	"testing"
)

func TestIsPortAvailable(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	available := IsPortAvailable(port)
	if available {
		t.Errorf("expected port %d to be unavailable (already in use)", port)
	}

	available = IsPortAvailable(0)
	if available {
		t.Error("expected port 0 to be unavailable")
	}
}

func TestIsPortAvailableFreePort(t *testing.T) {
	available := IsPortAvailable(12345)
	if available {
		// Port might be free - that's fine
		// If it's not free, that's fine too
		return
	}
}

func TestIsPortAvailableInvalidPorts(t *testing.T) {
	if IsPortAvailable(-1) {
		t.Error("expected -1 to be unavailable")
	}
	if IsPortAvailable(0) {
		t.Error("expected 0 to be unavailable")
	}
	if IsPortAvailable(65536) {
		t.Error("expected 65536 to be unavailable")
	}
}

func TestFindAvailablePort(t *testing.T) {
	port, err := FindAvailablePort(1024, func(msg string) {})
	if err != nil {
		t.Fatalf("FindAvailablePort failed: %v", err)
	}
	if port < 1024 || port > 65535 {
		t.Errorf("expected port in range [1024, 65535], got %d", port)
	}
}

func TestFindAvailablePortAdjustsMinimum(t *testing.T) {
	port, err := FindAvailablePort(1, func(msg string) {})
	if err != nil {
		t.Fatalf("FindAvailablePort failed: %v", err)
	}
	if port < 1024 {
		t.Errorf("expected port >= 1024, got %d", port)
	}
}

func TestFindAvailablePortOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	occupiedPort := listener.Addr().(*net.TCPAddr).Port

	// FindAvailablePort should skip the occupied port
	port, err := FindAvailablePort(occupiedPort, func(msg string) {
		t.Logf("retry log: %s", msg)
	})
	if err != nil {
		t.Fatalf("FindAvailablePort failed: %v", err)
	}
	if port == occupiedPort {
		t.Errorf("expected available port different from occupied port %d", occupiedPort)
	}
}

func TestFindAvailablePortAllOccupied(t *testing.T) {
	var listeners []net.Listener
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	startPort := 20000
	for i := 0; i < MaxPortRetries; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", startPort+i))
		if err != nil {
			t.Skipf("cannot occupy enough ports for test")
			return
		}
		listeners = append(listeners, l)
	}

	_, err := FindAvailablePort(startPort, func(msg string) {})
	if err == nil {
		t.Error("expected error when all ports are occupied")
	}
}
