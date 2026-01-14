# DevSSH

DevSSH 是一个通过 SSH 设置远程开发环境的工具，专注于在远程服务器上安装和管理 openvscode-server，并提供智能的端口转发功能。

## 功能特性

- **SSH 连接管理**：建立到远程服务器的 SSH 连接，支持多种认证方式（密钥、密码、SSH agent）
- **openvscode-server 安装**：在远程服务器上安装和管理 openvscode-server
- **智能进程检测**：多重检测机制确保不重复启动 openvscode 进程
- **本地下载上传**：openvscode 安装包在本地下载后通过 SSH 上传，不依赖远端网络
- **端口转发**：将远程端口通过 SSH 隧道转发到本地
- **SSH 配置集成**：自动读取和使用本地 SSH 配置文件（~/.ssh/config）
- **配置管理**：保存和管理主机配置、连接状态

## 安装

### 从源码构建

```bash
# 克隆仓库
git clone <repository-url>
cd DevSSH

# 构建（禁用CGO以避免依赖问题）
CGO_ENABLED=0 go build -o devssh ./cmd/devssh

# 安装到系统路径
sudo mv devssh /usr/local/bin/
```

### 依赖

- Go 1.19+
- 远程服务器需要支持 SSH 连接
- 无需本地 SSH 客户端（使用纯 Go 实现）

## 使用方法

### 基本连接和安装

```bash
# 连接到远程主机并安装 openvscode-server
devssh connect user@hostname

# 指定端口和密钥
devssh connect user@hostname --port 2222 --key ~/.ssh/id_rsa

# 使用 SSH 配置文件中的主机配置
devssh connect my-server-alias
```

### 单独安装 openvscode

```bash
# 在远程主机上安装 openvscode-server
devssh install user@hostname
```

### 端口转发

```bash
# 转发特定端口
devssh forward user@hostname --ports 3000,8080:80

# 自动检测并转发所有 Web 服务端口
devssh forward user@hostname --auto
```

### 管理连接和配置

```bash
# 列出活动连接
devssh list

# 停止连接
devssh stop <connection-id>

# 导入 SSH 配置文件中的主机
devssh import-ssh

# 列出 SSH 配置文件中的可用主机
devssh ssh-hosts
```

## 命令行选项

### connect 命令

```
Usage:
  devssh connect [host] [flags]

Flags:
  -h, --help             帮助信息
  -u, --user string      SSH 用户名（当主机不在SSH配置文件中时需要）
  -p, --port string      SSH 端口 (默认 "22")
      --key string       SSH 私钥路径
      --password string  SSH 密码
      --ide string       Web IDE 类型 (默认 "vscode")
      --install          如果不存在则安装 Web IDE (默认 true)
      --forward strings  要转发的端口 (例如: 3000, 8080:80)
      --auto             自动检测并转发 Web 服务端口
      --timeout int      SSH 连接超时时间（秒）(默认 30)
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
      --password string SSH 密码
      --ide string  Web IDE 类型 (默认 "vscode")
      --timeout int SSH 连接超时时间（秒）(默认 30)
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
      --password string SSH 密码
      --ports strings   要转发的端口 (例如: 3000, 8080:80)
      --auto          自动检测运行的服务并转发其端口
      --timeout int   SSH 连接超时时间（秒）(默认 30)
```

### 其他命令

```
# 列出活动连接
devssh list

# 停止连接
devssh stop <connection-id>

# 导入SSH配置文件中的主机
devssh import-ssh

# 列出SSH配置文件中的可用主机
devssh ssh-hosts
```

## 支持的 Web IDE

- **vscode**: openvscode-server（基于 Gitpod 的 openvscode 实现）
- **code-server**: 传统 code-server（通过 DevPod 适配器）

### openvscode-server 特性

- **本地下载上传**：安装包在本地下载后通过 SSH 上传，不依赖远端网络
- **智能进程检测**：多重检测机制确保不重复启动
- **自动版本管理**：支持指定版本，默认使用 v1.84.2
- **扩展安装**：支持安装 VSCode 扩展
- **配置管理**：支持自定义 VSCode 设置

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

1. **SSH 连接**: 使用 Go 的 `golang.org/x/crypto/ssh` 包建立 SSH 连接，支持从 SSH 配置文件读取配置
2. **IDE 安装**: 
   - 在本地下载 openvscode-server 安装包（使用 Go 的 http 模块）
   - 通过 SCP 协议（纯 Go 实现）上传到远程服务器
   - 在远程服务器解压安装
3. **进程管理**:
   - 多重检测机制检查 openvscode 进程是否已在运行
   - 使用 PID 文件管理进程生命周期
   - 智能启动脚本确保进程稳定运行
4. **端口检测**: 使用 `netstat`/`ss`/`lsof` 命令检测远程服务器上的监听端口
5. **端口转发**: 创建 SSH 隧道将远程端口转发到本地
6. **服务管理**: 在后台启动 Web IDE 服务并管理其生命周期

## 技术特点

- **纯 Go 实现**: SSH、SCP 都使用 Go 标准库，不依赖外部命令
- **复用 DevPod 逻辑**: 使用 DevPod 的 openvscode 安装逻辑，确保兼容性
- **本地下载上传**: 安装包在本地下载，不依赖远端网络环境
- **智能进程检测**: 5 种检测方法确保不重复启动 openvscode
- **SSH 配置集成**: 自动读取和使用本地 SSH 配置文件
- **模块化设计**: 清晰的包结构，易于扩展新的 IDE 类型
- **多认证支持**: 支持 SSH 密钥、密码、SSH agent 等多种认证方式

## 开发

### 项目结构

```
DevSSH/
├── cmd/
│   └── devssh/
│       ├── main.go          # 主命令行程序
│       └── agent.go         # Agent 管理功能（待实现）
├── pkg/
│   ├── ssh/                 # SSH 连接和隧道功能
│   │   ├── client.go        # SSH 客户端实现
│   │   ├── tunnel.go        # SSH 隧道实现
│   │   ├── config_parser.go # SSH 配置文件解析器
│   │   └── scp_client.go    # SCP 客户端（纯 Go 实现）
│   ├── ide/                 # Web IDE 安装和管理
│   │   ├── installer.go     # IDE 安装器接口
│   │   ├── openvscode_adapter.go  # openvscode-server 适配器
│   │   └── test_openvscode.go     # 测试文件
│   ├── tunnel/              # 端口转发和隧道管理
│   │   ├── manager.go       # 隧道管理器
│   │   └── port_scanner.go  # 端口扫描器
│   ├── config/              # 配置管理
│   │   └── config.go        # 配置文件管理
│   └── download/            # 下载管理
│       └── local_downloader.go  # 本地下载器
├── go.mod                   # Go 模块定义
├── README.md                # 项目文档
├── test_connection.sh       # 连接测试脚本
├── examples/                # 使用示例
└── docs/                    # 文档目录
```

### 构建和测试

```bash
# 构建（禁用 CGO）
CGO_ENABLED=0 go build -o devssh ./cmd/devssh

# 安装
sudo mv devssh /usr/local/bin/

# 运行测试
CGO_ENABLED=0 go test ./...

# 运行 openvscode 适配器测试
CGO_ENABLED=0 go run pkg/ide/test_openvscode.go
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
   - 如果使用 SSH 配置文件，确保主机配置正确

2. **openvscode 安装失败**
   - 检查本地网络连接（下载需要本地网络）
   - 确保有足够的磁盘空间存放缓存文件
   - 检查远程服务器的 `/tmp` 目录权限
   - 验证远程服务器有解压工具（tar）

3. **进程检测问题**
   - openvscode 可能已在运行但未被检测到
   - 检查远程服务器的进程检测工具（ps, lsof, ss）是否可用
   - 手动检查端口占用: `ssh user@host "lsof -i :8080"`

4. **端口转发失败**
   - 确保本地端口没有被占用
   - 检查远程服务是否在监听指定端口
   - 验证防火墙是否允许端口转发

### 调试模式

设置环境变量启用详细日志：

```bash
export DEVSSH_DEBUG=1
devssh connect user@hostname
```

### 手动清理

如果 openvscode 进程异常，可以手动清理：

```bash
# 清理远程服务器的 openvscode 进程和 PID 文件
ssh user@host "pkill -f openvscode; rm -f /tmp/openvscode-server-*.pid"
```

## 使用示例

### 示例 1：基本使用

```bash
# 连接到远程服务器并安装 openvscode
devssh connect user@example.com

# 使用完成后，在另一个终端停止连接
devssh stop <connection-id>
```

### 示例 2：使用 SSH 配置文件

```bash
# 首先配置 ~/.ssh/config
cat >> ~/.ssh/config << EOF
Host myserver
    HostName example.com
    User myuser
    Port 2222
    IdentityFile ~/.ssh/myserver_key
EOF

# 使用配置好的主机别名
devssh connect myserver
```

### 示例 3：高级使用

```bash
# 使用密码认证
devssh connect user@example.com --password "yourpassword"

# 同时转发其他应用端口（connect命令用--forward）
devssh connect user@example.com --forward 3000,8080

# 自动检测并转发所有Web服务端口
devssh connect user@example.com --auto

# 设置连接超时时间
devssh connect user@example.com --timeout 60

# 单独使用端口转发（forward命令用--ports）
devssh forward user@example.com --ports 3000,8080:80
```

### 示例 4：仅安装不连接

```bash
# 只安装 openvscode，不启动连接
devssh install user@example.com

# 稍后连接时使用已安装的 openvscode
devssh connect user@example.com --install false
```