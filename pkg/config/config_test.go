package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDevSSHDownloadURL_Default(t *testing.T) {
	url := GetDevSSHDownloadURL("0.1.8", "linux", "amd64")
	assert.Equal(t, "https://github.com/SilenWang/DevSSH/releases/download/0.1.8/devssh-0.1.8-linux-amd64", url)
}

func TestGetDevSSHDownloadURL_EnvOverride(t *testing.T) {
	t.Setenv(EnvDevSSHDownloadURL, "http://localhost:9999/devssh-{{version}}-{{os}}-{{arch}}")
	url := GetDevSSHDownloadURL("0.2.0", "darwin", "arm64")
	assert.Equal(t, "http://localhost:9999/devssh-0.2.0-darwin-arm64", url)
}

func TestGetDevSSHDownloadURL_EnvOverrideThenDefault(t *testing.T) {
	os.Setenv(EnvDevSSHDownloadURL, "http://localhost:9999/test")
	urlWithEnv := GetDevSSHDownloadURL("0.1.8", "linux", "amd64")
	assert.Equal(t, "http://localhost:9999/test", urlWithEnv)
	os.Unsetenv(EnvDevSSHDownloadURL)

	urlDefault := GetDevSSHDownloadURL("0.1.8", "linux", "amd64")
	assert.Equal(t, "https://github.com/SilenWang/DevSSH/releases/download/0.1.8/devssh-0.1.8-linux-amd64", urlDefault)
}

func TestNewConfig(t *testing.T) {
	c := NewConfig()
	require.NotNil(t, c)
	require.NotNil(t, c.Hosts)
}

func TestConfigAddGetRemoveHost(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	c := NewConfig()
	err := c.AddHost(HostConfig{Name: "test-host", Host: "192.168.1.1", Port: "22", Username: "testuser"})
	require.NoError(t, err)

	host, exists := c.GetHost("test-host")
	require.True(t, exists)
	assert.Equal(t, "192.168.1.1", host.Host)
	assert.Equal(t, "22", host.Port)
	assert.Equal(t, "testuser", host.Username)

	err = c.RemoveHost("test-host")
	require.NoError(t, err)
	_, exists = c.GetHost("test-host")
	assert.False(t, exists)
}

func TestConfigRemoveNonExistentHost(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	c := NewConfig()
	err := c.RemoveHost("nonexistent")
	assert.Error(t, err)
}

func TestConfigPersistAndLoad(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	c := NewConfig()
	c.AddHost(HostConfig{Name: "persist-host", Host: "10.0.0.1", Port: "2222", Username: "admin"})

	loaded, err := Load()
	require.NoError(t, err)

	host, exists := loaded.GetHost("persist-host")
	require.True(t, exists)
	assert.Equal(t, "10.0.0.1", host.Host)
}

func TestGetConfigDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	dir, err := GetConfigDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(homeDir, ".config", "devssh"), dir)
}

func TestListHosts(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	c := NewConfig()
	c.AddHost(HostConfig{Name: "host1", Host: "1.1.1.1", Port: "22", Username: "u1"})
	c.AddHost(HostConfig{Name: "host2", Host: "2.2.2.2", Port: "22", Username: "u2"})

	hosts := c.ListHosts()
	assert.Len(t, hosts, 2)
}
