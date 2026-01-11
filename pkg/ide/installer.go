package ide

import (
	"fmt"
	"strings"

	"devssh/pkg/ssh"
)

type IDE string

const (
	VSCode     IDE = "vscode"
	CodeServer IDE = "code-server"
	Jupyter    IDE = "jupyter"
	Theia      IDE = "theia"
)

type Installer struct {
	sshClient *ssh.Client
	ideType   IDE
}

func NewInstaller(sshClient *ssh.Client, ideType IDE) *Installer {
	return &Installer{
		sshClient: sshClient,
		ideType:   ideType,
	}
}

func (i *Installer) Install() error {
	if !i.sshClient.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	// 检查是否已经安装
	installed, err := i.IsInstalled()
	if err != nil {
		return fmt.Errorf("failed to check if IDE is installed: %w", err)
	}

	if installed {
		fmt.Printf("%s is already installed\n", i.ideType)
		return nil
	}

	fmt.Printf("Installing %s...\n", i.ideType)

	switch i.ideType {
	case VSCode:
		return i.installVSCode()
	case CodeServer:
		return i.installCodeServer()
	case Jupyter:
		return i.installJupyter()
	case Theia:
		return i.installTheia()
	default:
		return fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *Installer) IsInstalled() (bool, error) {
	switch i.ideType {
	case VSCode, CodeServer:
		// 检查 openvscode-server 是否安装
		checkCmd := "test -f ~/.openvscode-server/bin/openvscode-server && echo installed"
		output, err := i.sshClient.RunCommand(checkCmd)
		if err != nil {
			return false, nil
		}
		return strings.Contains(output, "installed"), nil

	case Jupyter:
		// 检查 jupyter 是否安装
		output, err := i.sshClient.RunCommand("which jupyter")
		if err != nil {
			return false, nil
		}
		return strings.TrimSpace(output) != "", nil

	case Theia:
		// 检查 theia 是否安装
		output, err := i.sshClient.RunCommand("which theia")
		if err != nil {
			return false, nil
		}
		return strings.TrimSpace(output) != "", nil

	default:
		return false, fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *Installer) Start(port int) error {
	switch i.ideType {
	case VSCode, CodeServer:
		return i.startCodeServer(port)
	case Jupyter:
		return i.startJupyter(port)
	case Theia:
		return i.startTheia(port)
	default:
		return fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *Installer) installVSCode() error {
	// 安装 openvscode-server (VSCode 的 Web 版本)
	installScript := `
#!/bin/bash
set -e

# 检查是否已经安装
if [ -f ~/.openvscode-server/bin/openvscode-server ]; then
	echo "openvscode-server is already installed"
	exit 0
fi

# 检测系统架构
ARCH=$(uname -m)
case $ARCH in
	x86_64|amd64)
		ARCH="x64"
		;;
	aarch64|arm64)
		ARCH="arm64"
		;;
	*)
		echo "Unsupported architecture: $ARCH"
		exit 1
		;;
esac

# 设置版本
VERSION="v1.84.2"

# 下载URL
DOWNLOAD_URL="https://github.com/gitpod-io/openvscode-server/releases/download/openvscode-server-${VERSION}/openvscode-server-${VERSION}-linux-${ARCH}.tar.gz"

echo "Downloading openvscode-server ${VERSION} for ${ARCH}..."

# 创建目录
mkdir -p ~/.openvscode-server

# 下载并解压
if command -v curl &> /dev/null; then
	curl -L "$DOWNLOAD_URL" | tar -xz -C ~/.openvscode-server --strip-components=1
elif command -v wget &> /dev/null; then
	wget -qO- "$DOWNLOAD_URL" | tar -xz -C ~/.openvscode-server --strip-components=1
else
	echo "Error: curl or wget is required"
	exit 1
fi

if [ $? -eq 0 ]; then
	echo "openvscode-server installed successfully at ~/.openvscode-server"
else
	echo "Failed to install openvscode-server"
	exit 1
fi
`

	_, err := i.sshClient.RunCommand(installScript)
	return err
}

func (i *Installer) installCodeServer() error {
	// code-server 和 VSCode 使用相同的安装脚本
	return i.installVSCode()
}

func (i *Installer) installJupyter() error {
	installScript := `
set -e
# 安装 Python 和 pip
if ! command -v python3 &> /dev/null; then
	if command -v apt-get &> /dev/null; then
		apt-get update
		apt-get install -y python3 python3-pip
	elif command -v yum &> /dev/null; then
		yum install -y python3 python3-pip
	elif command -v apk &> /dev/null; then
		apk add --no-cache python3 py3-pip
	fi
fi

# 安装 jupyter
python3 -m pip install --upgrade pip
python3 -m pip install jupyter
`

	_, err := i.sshClient.RunCommand(installScript)
	return err
}

func (i *Installer) installTheia() error {
	installScript := `
set -e
# 安装 Node.js 和 npm
if ! command -v node &> /dev/null; then
	if command -v apt-get &> /dev/null; then
		apt-get update
		apt-get install -y nodejs npm
	elif command -v yum &> /dev/null; then
		yum install -y nodejs npm
	elif command -v apk &> /dev/null; then
		apk add --no-cache nodejs npm
	fi
fi

# 安装 theia
npm install -g @theia/cli
`

	_, err := i.sshClient.RunCommand(installScript)
	return err
}

func (i *Installer) startCodeServer(port int) error {
	// 在后台启动 openvscode-server
	cmd := fmt.Sprintf("nohup ~/.openvscode-server/bin/openvscode-server --host 0.0.0.0 --port %d --without-connection-token > /tmp/openvscode.log 2>&1 &", port)
	_, err := i.sshClient.RunCommand(cmd)
	return err
}

func (i *Installer) startJupyter(port int) error {
	// 在后台启动 jupyter
	cmd := fmt.Sprintf("nohup jupyter notebook --port=%d --ip=0.0.0.0 --no-browser > /tmp/jupyter.log 2>&1 &", port)
	_, err := i.sshClient.RunCommand(cmd)
	return err
}

func (i *Installer) startTheia(port int) error {
	// 在后台启动 theia
	cmd := fmt.Sprintf("nohup theia start --port=%d --hostname=0.0.0.0 > /tmp/theia.log 2>&1 &", port)
	_, err := i.sshClient.RunCommand(cmd)
	return err
}

func (i *Installer) GetDefaultPort() int {
	switch i.ideType {
	case VSCode, CodeServer:
		return 8080
	case Jupyter:
		return 8888
	case Theia:
		return 3000
	default:
		return 8080
	}
}

func (i *Installer) GetName() string {
	return string(i.ideType)
}
