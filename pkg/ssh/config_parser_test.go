package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSSHConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write SSH config: %v", err)
	}
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
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	host, exists := hosts["myserver"]
	if !exists {
		t.Fatal("expected host 'myserver' to exist")
	}
	if host.HostName != "192.168.1.100" {
		t.Errorf("expected HostName 192.168.1.100, got %s", host.HostName)
	}
	if host.User != "ubuntu" {
		t.Errorf("expected User ubuntu, got %s", host.User)
	}
	if host.Port != "2222" {
		t.Errorf("expected Port 2222, got %s", host.Port)
	}
	if host.IdentityFile == "" || (!filepath.IsAbs(host.IdentityFile) && host.IdentityFile != "~/.ssh/mykey") {
		t.Errorf("expected IdentityFile to be set, got %s", host.IdentityFile)
	}
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
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}
	if _, exists := hosts["host1"]; !exists {
		t.Error("expected host1")
	}
	if _, exists := hosts["host2"]; !exists {
		t.Error("expected host2")
	}
}

func TestParseWildcardHost(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host *
    User defaultuser
    IdentityFile ~/.ssh/default_key

Host specific
    HostName 10.0.0.1
    User specificuser
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if _, exists := hosts["*"]; exists {
		t.Error("wildcard host '*' should not appear in parsed hosts")
	}

	host, exists := hosts["specific"]
	if !exists {
		t.Fatal("expected host 'specific' to exist")
	}
	if host.User != "specificuser" {
		t.Errorf("expected User specificuser, got %s", host.User)
	}
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
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if _, exists := hosts["*.example.com"]; exists {
		t.Error("pattern host should not appear in parsed hosts")
	}
	if _, exists := hosts["?"]; exists {
		t.Error("pattern host '?' should not appear in parsed hosts")
	}
}

func TestGetHostFound(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host foundhost
    HostName 10.0.0.1
    User testuser
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	host, err := parser.GetHost("foundhost")
	if err != nil {
		t.Fatalf("GetHost failed: %v", err)
	}
	if host.HostName != "10.0.0.1" {
		t.Errorf("expected HostName 10.0.0.1, got %s", host.HostName)
	}
}

func TestGetHostNotFound(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host existing
    HostName 10.0.0.1
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	_, err := parser.GetHost("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent host")
	}
}

func TestGetHostSpecialPattern(t *testing.T) {
	configPath := writeSSHConfig(t, `
Host valid
    HostName 10.0.0.1
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	_, err := parser.GetHost("*")
	if err == nil {
		t.Fatal("expected error for special pattern")
	}
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
	if err != nil {
		t.Fatalf("ListHosts failed: %v", err)
	}
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}
}

func TestParseEmptyConfig(t *testing.T) {
	configPath := writeSSHConfig(t, "")
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts, got %d", len(hosts))
	}
}

func TestParseCommentsAndBlankLines(t *testing.T) {
	configPath := writeSSHConfig(t, `
# This is a comment

Host myhost
    HostName 10.0.0.1

# Another comment
    User myuser
`)
	parser := NewSSHConfigParser().WithConfigPath(configPath)
	hosts, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(hosts))
	}
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
	if cfg.Host != "real-host.example.com" {
		t.Errorf("expected Host real-host.example.com, got %s", cfg.Host)
	}
	if cfg.Port != "2222" {
		t.Errorf("expected Port 2222, got %s", cfg.Port)
	}
	if cfg.Username != "admin" {
		t.Errorf("expected Username admin, got %s", cfg.Username)
	}
	if cfg.KeyPath != "/path/to/key" {
		t.Errorf("expected KeyPath /path/to/key, got %s", cfg.KeyPath)
	}
}

func TestSSHHostConfigGetHostConfigForSSH_FallbackHost(t *testing.T) {
	h := &SSHHostConfig{
		Host: "alias-only",
		Port: "22",
	}
	cfg := h.GetHostConfigForSSH()
	if cfg.Host != "alias-only" {
		t.Errorf("expected Host alias-only (fallback), got %s", cfg.Host)
	}
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
		got := isSpecialHostPattern(tt.name)
		if got != tt.expected {
			t.Errorf("isSpecialHostPattern(%q) = %v, want %v", tt.name, got, tt.expected)
		}
	}
}
