package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDevSSHDownloadURL_Default(t *testing.T) {
	url := GetDevSSHDownloadURL("0.1.8", "linux", "amd64")
	expected := "https://github.com/SilenWang/DevSSH/releases/download/0.1.8/devssh-0.1.8-linux-amd64"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestGetDevSSHDownloadURL_EnvOverride(t *testing.T) {
	os.Setenv(EnvDevSSHDownloadURL, "http://localhost:9999/devssh-{{version}}-{{os}}-{{arch}}")
	defer os.Unsetenv(EnvDevSSHDownloadURL)

	url := GetDevSSHDownloadURL("0.2.0", "darwin", "arm64")
	expected := "http://localhost:9999/devssh-0.2.0-darwin-arm64"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestGetDevSSHDownloadURL_DefaultRestoreAfterEnv(t *testing.T) {
	os.Setenv(EnvDevSSHDownloadURL, "http://localhost:9999/test")
	_ = GetDevSSHDownloadURL("1.0", "linux", "amd64")
	os.Unsetenv(EnvDevSSHDownloadURL)

	url := GetDevSSHDownloadURL("0.1.8", "linux", "amd64")
	expected := "https://github.com/SilenWang/DevSSH/releases/download/0.1.8/devssh-0.1.8-linux-amd64"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestNewConfig(t *testing.T) {
	c := NewConfig()
	if c == nil {
		t.Fatal("expected non-nil config")
	}
	if c.Hosts == nil {
		t.Fatal("expected non-nil hosts map")
	}
}

func TestConfigAddGetRemoveHost(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	c := NewConfig()
	err := c.AddHost(HostConfig{
		Name:     "test-host",
		Host:     "192.168.1.1",
		Port:     "22",
		Username: "testuser",
	})
	if err != nil {
		t.Fatalf("AddHost failed: %v", err)
	}

	host, exists := c.GetHost("test-host")
	if !exists {
		t.Fatal("expected host to exist")
	}
	if host.Host != "192.168.1.1" {
		t.Errorf("expected Host 192.168.1.1, got %s", host.Host)
	}
	if host.Port != "22" {
		t.Errorf("expected Port 22, got %s", host.Port)
	}
	if host.Username != "testuser" {
		t.Errorf("expected Username testuser, got %s", host.Username)
	}

	err = c.RemoveHost("test-host")
	if err != nil {
		t.Fatalf("RemoveHost failed: %v", err)
	}
	_, exists = c.GetHost("test-host")
	if exists {
		t.Fatal("expected host to be removed")
	}
}

func TestConfigRemoveNonExistentHost(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	c := NewConfig()
	err := c.RemoveHost("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent host")
	}
}

func TestConfigPersistAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	c := NewConfig()
	c.AddHost(HostConfig{Name: "persist-host", Host: "10.0.0.1", Port: "2222", Username: "admin"})

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	host, exists := loaded.GetHost("persist-host")
	if !exists {
		t.Fatal("expected host to exist after reload")
	}
	if host.Host != "10.0.0.1" {
		t.Errorf("expected Host 10.0.0.1, got %s", host.Host)
	}
}

func TestConfigGetConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	dir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir failed: %v", err)
	}
	expected := filepath.Join(tmpDir, ".config", "devssh")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestListHosts(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	c := NewConfig()
	c.AddHost(HostConfig{Name: "host1", Host: "1.1.1.1", Port: "22", Username: "u1"})
	c.AddHost(HostConfig{Name: "host2", Host: "2.2.2.2", Port: "22", Username: "u2"})

	hosts := c.ListHosts()
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}
}
