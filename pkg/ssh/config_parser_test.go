package ssh

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSSHConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)
	return configPath
}

func TestParseSimpleHost(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host myserver
    HostName 192.168.1.100
    User ubuntu
    Port 2222
    IdentityFile ~/.ssh/mykey
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	require.NoError(t, err)

	host, exists := hosts["myserver"]
	require.True(t, exists)
	assert.Equal(t, "192.168.1.100", host.HostName)
	assert.Equal(t, "ubuntu", host.User)
	assert.Equal(t, "2222", host.Port)
	assert.NotEmpty(t, host.IdentityFile)
}

func TestParseMultipleHosts(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host host1
    HostName 10.0.0.1
    User user1

Host host2
    HostName 10.0.0.2
    User user2
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	require.NoError(t, err)
	assert.Len(t, hosts, 2)
	assert.Contains(t, hosts, "host1")
	assert.Contains(t, hosts, "host2")
}

func TestParseWildcardHost(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host *
    User defaultuser

Host specific
    HostName 10.0.0.1
    User specificuser
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	require.NoError(t, err)

	assert.NotContains(t, hosts, "*", "wildcard host should not appear")

	host, exists := hosts["specific"]
	require.True(t, exists)
	assert.Equal(t, "specificuser", host.User)
}

func TestParsePatternHost(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host *.example.com
    User exampleuser

Host ?
    User singlechar
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	require.NoError(t, err)

	assert.NotContains(t, hosts, "*.example.com")
	assert.NotContains(t, hosts, "?")
}

func TestGetHostFound(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host foundhost
    HostName 10.0.0.1
    User testuser
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	host, err := parser.GetHost("foundhost")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1", host.HostName)
}

func TestGetHostNotFound(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host existing
    HostName 10.0.0.1
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	_, err := parser.GetHost("nonexistent")
	assert.Error(t, err)
}

func TestGetHostSpecialPattern(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host valid
    HostName 10.0.0.1
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	_, err := parser.GetHost("*")
	assert.Error(t, err)
}

func TestListHosts(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host alpha
    HostName 10.0.0.1
Host beta
    HostName 10.0.0.2
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.ListHosts()
	require.NoError(t, err)
	assert.Len(t, hosts, 2)
}

func TestParseEmptyConfig(t *testing.T) {
	configPath := writeSSHConfig(t, "")
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	require.NoError(t, err)
	assert.Empty(t, hosts)
}

func TestParseCommentsAndBlankLines(t *testing.T) {
	configPath := writeSSHConfig(t, `
# Comment
Host myhost
    HostName 10.0.0.1
    User myuser
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	require.NoError(t, err)
	assert.Len(t, hosts, 1)
}

func TestSSHHostConfigGetHostConfigForSSH(t *testing.T) {
	h := &SSHHostConfig{
		Host:         "alias",
		HostName:     "real-host.example.com",
		User:         "admin",
		Port:         "2222",
		IdentityFile: "/path/to/key",
	}
	cfg := h.GetHostConfigForSSH()
	assert.Equal(t, "real-host.example.com", cfg.Host)
	assert.Equal(t, "2222", cfg.Port)
	assert.Equal(t, "admin", cfg.Username)
	assert.Equal(t, "/path/to/key", cfg.KeyPath)
}

func TestSSHHostConfigGetHostConfigForSSH_Fallback(t *testing.T) {
	h := &SSHHostConfig{Host: "alias-only", Port: "22"}
	cfg := h.GetHostConfigForSSH()
	assert.Equal(t, "alias-only", cfg.Host)
}

func TestIsSpecialHostPattern(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"*", true},
		{"*.example.com", true},
		{"?", true},
		{"???", true},
		{"!host", true},
		{"normal-host", false},
		{"web-1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isSpecialHostPattern(tt.name))
		})
	}
}
