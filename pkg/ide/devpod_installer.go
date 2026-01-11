package ide

import (
	"fmt"
	"os"
	"strings"

	"github.com/loft-sh/devpod/pkg/config"
	"github.com/loft-sh/log"
	"github.com/sirupsen/logrus"
	"github.com/sylens/project/DevSSH/pkg/ssh"
)

type DevPodInstaller struct {
	sshClient *ssh.Client
	ideType   IDE
	values    map[string]config.OptionValue
	logger    log.Logger
}

func NewDevPodInstaller(sshClient *ssh.Client, ideType IDE) *DevPodInstaller {
	values := map[string]config.OptionValue{
		"FORWARD_PORTS": {Value: "true"},
		"OPEN":          {Value: "false"},
		"BIND_ADDRESS":  {Value: ""},
		"VERSION":       {Value: "v1.84.2"},
	}

	// 创建一个简单的logger
	logger := log.NewStreamLogger(os.Stdout, os.Stderr, logrus.InfoLevel)

	return &DevPodInstaller{
		sshClient: sshClient,
		ideType:   ideType,
		values:    values,
		logger:    logger,
	}
}

func (i *DevPodInstaller) Install() error {
	if !i.sshClient.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	switch i.ideType {
	case VSCode, CodeServer:
		return i.installOpenVSCode()
	case Jupyter:
		return i.installJupyter()
	case Theia:
		return i.installTheia()
	default:
		return fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *DevPodInstaller) installOpenVSCode() error {
	// 由于DevPod的openvscode安装逻辑是在本地运行的，
	// 我们需要通过SSH在远程服务器上执行安装脚本
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

func (i *DevPodInstaller) installJupyter() error {
	// 使用DevPod的Jupyter安装逻辑
	installScript := `
#!/bin/bash
set -e

# 检查Python3是否安装
if ! command -v python3 &> /dev/null; then
	echo "Installing Python3..."
	if command -v apt-get &> /dev/null; then
		apt-get update
		apt-get install -y python3 python3-pip python3-venv
	elif command -v yum &> /dev/null; then
		yum install -y python3 python3-pip
	elif command -v apk &> /dev/null; then
		apk add --no-cache python3 py3-pip
	else
		echo "Unsupported package manager"
		exit 1
	fi
fi

# 创建虚拟环境
python3 -m venv /tmp/jupyter-venv
source /tmp/jupyter-venv/bin/activate

# 安装jupyter
pip install --upgrade pip
pip install jupyter

echo "Jupyter installed successfully"
`

	_, err := i.sshClient.RunCommand(installScript)
	return err
}

func (i *DevPodInstaller) installTheia() error {
	// 使用DevPod的Theia安装逻辑
	installScript := `
#!/bin/bash
set -e

# 检查Node.js是否安装
if ! command -v node &> /dev/null; then
	echo "Installing Node.js..."
	if command -v apt-get &> /dev/null; then
		curl -fsSL https://deb.nodesource.com/setup_18.x | bash -
		apt-get install -y nodejs
	elif command -v yum &> /dev/null; then
		curl -fsSL https://rpm.nodesource.com/setup_18.x | bash -
		yum install -y nodejs
	elif command -v apk &> /dev/null; then
		apk add --no-cache nodejs npm
	else
		echo "Unsupported package manager"
		exit 1
	fi
fi

# 安装theia
npm install -g @theia/cli @theia/application-package

echo "Theia installed successfully"
`

	_, err := i.sshClient.RunCommand(installScript)
	return err
}

func (i *DevPodInstaller) Start(port int) error {
	switch i.ideType {
	case VSCode, CodeServer:
		return i.startOpenVSCode(port)
	case Jupyter:
		return i.startJupyter(port)
	case Theia:
		return i.startTheia(port)
	default:
		return fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *DevPodInstaller) startOpenVSCode(port int) error {
	// 启动openvscode-server
	cmd := fmt.Sprintf("nohup ~/.openvscode-server/bin/openvscode-server --host 0.0.0.0 --port %d --without-connection-token > /tmp/openvscode.log 2>&1 &", port)
	_, err := i.sshClient.RunCommand(cmd)
	return err
}

func (i *DevPodInstaller) startJupyter(port int) error {
	cmd := fmt.Sprintf("nohup /tmp/jupyter-venv/bin/jupyter notebook --port=%d --ip=0.0.0.0 --no-browser > /tmp/jupyter.log 2>&1 &", port)
	_, err := i.sshClient.RunCommand(cmd)
	return err
}

func (i *DevPodInstaller) startTheia(port int) error {
	cmd := fmt.Sprintf("nohup theia start /home/%s --hostname=0.0.0.0 --port=%d > /tmp/theia.log 2>&1 &", i.sshClient.GetConfig().Username, port)
	_, err := i.sshClient.RunCommand(cmd)
	return err
}

func (i *DevPodInstaller) IsInstalled() (bool, error) {
	switch i.ideType {
	case VSCode, CodeServer:
		// 检查openvscode-server是否安装
		checkCmd := "test -f ~/.openvscode-server/bin/openvscode-server && echo installed"
		output, err := i.sshClient.RunCommand(checkCmd)
		if err != nil {
			return false, nil
		}
		return strings.Contains(output, "installed"), nil
	case Jupyter:
		output, err := i.sshClient.RunCommand("which jupyter")
		if err != nil {
			return false, nil
		}
		return output != "", nil
	case Theia:
		output, err := i.sshClient.RunCommand("which theia")
		if err != nil {
			return false, nil
		}
		return output != "", nil
	default:
		return false, fmt.Errorf("unsupported IDE: %s", i.ideType)
	}
}

func (i *DevPodInstaller) GetDefaultPort() int {
	switch i.ideType {
	case VSCode, CodeServer:
		return 10800 // openvscode默认端口
	case Jupyter:
		return 8888
	case Theia:
		return 3000
	default:
		return 8080
	}
}

func (i *DevPodInstaller) GetName() string {
	return string(i.ideType)
}

func (i *DevPodInstaller) SetLogger(logger log.Logger) {
	i.logger = logger
}
