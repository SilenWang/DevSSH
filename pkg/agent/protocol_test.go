package agent

import (
	"testing"
)

func TestIDETypeConstants(t *testing.T) {
	if IDETypeVSCode != "vscode" {
		t.Errorf("expected IDETypeVSCode to be 'vscode', got %q", IDETypeVSCode)
	}
	if IDETypeCodeServer != "code-server" {
		t.Errorf("expected IDETypeCodeServer to be 'code-server', got %q", IDETypeCodeServer)
	}
}

func TestIDEConfigDefaults(t *testing.T) {
	cfg := IDEConfig{
		Type: IDETypeVSCode,
		Port: 10081,
	}
	if cfg.Type != "vscode" {
		t.Errorf("expected Type vscode, got %s", cfg.Type)
	}
	if cfg.Version != "" {
		t.Errorf("expected empty Version, got %s", cfg.Version)
	}
	if cfg.Port != 10081 {
		t.Errorf("expected Port 10081, got %d", cfg.Port)
	}
	if cfg.Options != nil {
		t.Errorf("expected nil Options, got %v", cfg.Options)
	}
}

func TestIDEStatus(t *testing.T) {
	status := IDEStatus{
		Type:   IDETypeVSCode,
		Status: "running",
		Port:   10081,
		PID:    12345,
		URL:    "http://localhost:10081",
		Config: IDEConfig{
			Type: IDETypeVSCode,
			Port: 10081,
		},
	}
	if status.Status != "running" {
		t.Errorf("expected Status 'running', got %s", status.Status)
	}
	if status.PID != 12345 {
		t.Errorf("expected PID 12345, got %d", status.PID)
	}
	if status.URL != "http://localhost:10081" {
		t.Errorf("expected URL 'http://localhost:10081', got %s", status.URL)
	}
}
