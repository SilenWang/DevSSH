package tunnel

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPortAvailable(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	assert.False(t, IsPortAvailable(port), "occupied port should be unavailable")
	assert.False(t, IsPortAvailable(0), "port 0 should be unavailable")
}

func TestIsPortAvailableInvalidPorts(t *testing.T) {
	assert.False(t, IsPortAvailable(-1))
	assert.False(t, IsPortAvailable(0))
	assert.False(t, IsPortAvailable(65536))
}

func TestFindAvailablePort(t *testing.T) {
	port, err := FindAvailablePort(1024, func(msg string) {})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, 1024)
	assert.LessOrEqual(t, port, 65535)
}

func TestFindAvailablePortAdjustsMinimum(t *testing.T) {
	port, err := FindAvailablePort(1, func(msg string) {})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, 1024)
}

func TestFindAvailablePortOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer listener.Close()

	occupiedPort := listener.Addr().(*net.TCPAddr).Port

	port, err := FindAvailablePort(occupiedPort, func(msg string) {
		t.Logf("retry: %s", msg)
	})
	require.NoError(t, err)
	assert.NotEqual(t, occupiedPort, port, "should skip occupied port")
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
			t.Skip("cannot occupy enough ports")
			return
		}
		listeners = append(listeners, l)
	}

	_, err := FindAvailablePort(startPort, func(msg string) {})
	assert.Error(t, err)
}
