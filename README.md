# DevSSH

DevSSH 是一个通过 SSH 设置远程开发环境的工具，可以安装 Web IDE（如 VSCode）并将远程端口转发到本地机器。

## 功能特性

- **SSH 连接管理**：建立到远程服务器的 SSH 连接
- **Web IDE 安装**：在远程服务器上安装 VSCode、code-server、Jupyter 等 Web IDE
- **端口自动识别**：自动检测远程服务器上运行的服务端口
- **端口转发**：将远程端口通过 SSH 隧道转发到本地
- **配置管理**：保存和管理主机配置、连接状态

## 安装

### 从源码构建

```bash
# 克隆仓库
git clone <repository-url>
cd DevSSH

# 构建
go build -o devssh ./cmd/devssh

# 安装到系统路径
sudo mv devssh /usr/local/bin/
```

### 依赖

- Go 1.19+
- SSH 客户端
- 远程服务器需要支持 SSH 连接

## 使用方法

### 基本连接

```bash
# 连接到远程主机并安装 VSCode
devssh connect user@hostname

# 指定端口和密钥
devssh connect user@hostname --port 2222 --key ~/.ssh/id_rsa

# 安装特定 IDE
devssh connect user@hostname --ide code-server

# 自动检测并转发端口
devssh connect user@hostname --auto
```

### 单独安装 IDE

```bash
# 在远程主机上安装 Web IDE
devssh install user@hostname --ide vscode
```

### 端口转发

```bash
# 转发特定端口
devssh forward user@hostname --ports 3000,8080:80

# 自动检测并转发所有 Web 服务端口
devssh forward user@hostname --auto
```

### 管理连接

```bash
# 列出活动连接
devssh list

# 停止连接
devssh stop <connection-id>
```

## 命令行选项

### connect 命令

```
Usage:
  devssh connect [host] [flags]

Flags:
  -h, --help             帮助信息
  -u, --user string      SSH 用户名
  -p, --port string      SSH 端口 (默认 "22")
      --key string       SSH 私钥路径
      --ide string       Web IDE 类型 (默认 "vscode")
      --install          如果不存在则安装 Web IDE (默认 true)
      --forward strings  要转发的端口 (例如: 3000, 8080:80)
```

### install 命令

```
Usage:
  devssh install [host] [flags]

Flags:
  -h, --help        帮助信息
  -u, --user string SSH 用户名
  -p, --port string SSH 端口 (默认 "22")
      --key string  SSH 私钥路径
      --ide string  Web IDE 类型 (默认 "vscode")
```

### forward 命令

```
Usage:
  devssh forward [host] [flags]

Flags:
  -h, --help          帮助信息
  -u, --user string   SSH 用户名
  -p, --port string   SSH 端口 (默认 "22")
      --key string    SSH 私钥路径
      --ports strings 要转发的端口 (例如: 3000, 8080:80)
      --auto          自动检测运行的服务并转发其端口
```

## 支持的 Web IDE

- **vscode** / **code-server**: Visual Studio Code 的 Web 版本
- **jupyter**: Jupyter Notebook
- **theia**: Theia IDE

## 配置

DevSSH 的配置文件位于 `~/.config/devssh/config.json`，包含以下内容：

```json
{
  "hosts": {
    "my-server": {
      "name": "my-server",
      "host": "example.com",
      "port": "22",
      "username": "user",
      "key_path": "~/.ssh/id_rsa"
    }
  },
  "connections": {
    "connection-id": {
      "id": "connection-id",
      "host": "example.com",
      "port": "22",
      "username": "user",
      "ide": "vscode",
      "local_port": 8080,
      "started_at": "2024-01-11T10:30:00Z"
    }
  }
}
```

## 工作原理

1. **SSH 连接**: 使用 Go 的 `golang.org/x/crypto/ssh` 包建立 SSH 连接
2. **IDE 安装**: 通过 SSH 在远程服务器上执行安装脚本
3. **端口检测**: 使用 `netstat`/`ss` 命令检测远程服务器上的监听端口
4. **端口转发**: 创建 SSH 隧道将远程端口转发到本地
5. **服务管理**: 在后台启动 Web IDE 服务并管理其生命周期

## 开发

### 项目结构

```
DevSSH/
├── cmd/
│   └── devssh/
│       └── main.go          # 主命令行程序
├── pkg/
│   ├── ssh/                 # SSH 连接和隧道功能
│   │   ├── client.go
│   │   └── tunnel.go
│   ├── ide/                 # Web IDE 安装和管理
│   │   └── installer.go
│   ├── tunnel/              # 端口转发和隧道管理
│   │   ├── manager.go
│   │   └── port_scanner.go
│   └── config/              # 配置管理
│       └── config.go
├── go.mod
└── README.md
```

### 构建和测试

```bash
# 运行测试
go test ./...

# 构建
go build ./cmd/devssh

# 安装
go install ./cmd/devssh
```

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

## 故障排除

### 常见问题

1. **SSH 连接失败**
   - 检查 SSH 密钥权限: `chmod 600 ~/.ssh/id_rsa`
   - 验证 SSH 服务在远程服务器上运行
   - 检查防火墙设置

2. **IDE 安装失败**
   - 确保远程服务器有网络连接
   - 检查是否有足够的磁盘空间
   - 查看远程服务器的系统日志

3. **端口转发失败**
   - 确保本地端口没有被占用
   - 检查远程服务是否在监听指定端口
   - 验证防火墙是否允许端口转发

### 调试模式

设置环境变量启用详细日志：

```bash
export DEVSSH_DEBUG=1
devssh connect user@hostname
```