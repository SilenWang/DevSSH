package agent

type IDEType string

const (
	IDETypeVSCode     IDEType = "vscode"
	IDETypeCodeServer IDEType = "code-server"
)

type IDEConfig struct {
	Type    IDEType           `json:"type"`
	Version string            `json:"version,omitempty"`
	Port    int               `json:"port"`
	Options map[string]string `json:"options,omitempty"`
}

type IDEStatus struct {
	Type    IDEType   `json:"type"`
	Status  string    `json:"status"`
	Port    int       `json:"port"`
	PID     int       `json:"pid,omitempty"`
	URL     string    `json:"url,omitempty"`
	Started string    `json:"started,omitempty"`
	Config  IDEConfig `json:"config"`
}
