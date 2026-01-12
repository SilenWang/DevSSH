# SSH配置文件集成功能

## 概述

DevSSH现在支持读取用户的SSH配置文件（~/.ssh/config），以便快速连接到已配置的远程主机。这使得用户可以使用他们已经熟悉的SSH配置，无需重复输入用户名、端口、密钥路径等信息。

## 新功能

### 1. SSH配置文件解析器
- 自动解析标准的SSH配置文件格式
- 支持主机别名、主机名、用户名、端口、身份文件等配置
- 自动展开波浪号路径（~/.ssh/id_rsa）
- 支持续行和注释

### 2. 快速连接
- 当使用`devssh connect [host]`时，如果`[host]`在SSH配置文件中定义，将自动使用配置文件中的设置
- 优先使用SSH配置文件中的身份文件
- 保持向后兼容：如果主机不在SSH配置文件中，使用传统方式连接

### 3. 新命令

#### `devssh ssh-hosts`
列出SSH配置文件中的所有主机别名。

```bash
devssh ssh-hosts
```

#### `devssh import-ssh`
将SSH配置文件中的主机导入到DevSSH的配置中。

```bash
devssh import-ssh
```

## 使用示例

### 1. 查看SSH配置文件中的主机
```bash
devssh ssh-hosts
```

输出：
```
Hosts from SSH config file:
  Ironman_Peanut
  AMD
  AMD_Remote
  Ironman_QNAP
  Ironman
```

### 2. 快速连接到SSH配置文件中的主机
```bash
devssh connect AMD
```

这将自动使用SSH配置文件中AMD主机的配置：
- 主机名：192.168.8.102
- 用户名：silen
- 端口：22
- 身份文件：/home/sylens/.ssh/amd

### 3. 导入SSH主机到DevSSH配置
```bash
devssh import-ssh
```

输出：
```
Successfully imported 5 hosts from SSH config file
```

## 配置优先级

当连接主机时，DevSSH按以下顺序使用配置：

1. **SSH配置文件**：如果主机在~/.ssh/config中定义，使用该配置
2. **命令行参数**：如果提供了命令行参数（如--user、--port等），将覆盖SSH配置文件中的设置
3. **默认值**：使用默认值（端口22等）

## SSH配置文件示例

```ssh
Host myserver
    HostName server.example.com
    User myuser
    Port 2222
    IdentityFile ~/.ssh/myserver_key

Host internal
    HostName 192.168.1.100
    User admin
    IdentityFile ~/.ssh/internal_rsa

Host *
    PubkeyAcceptedKeyTypes +ssh-rsa
    HostKeyAlgorithms +ssh-rsa
```

## 注意事项

1. 通配符主机（如`Host *`）不会被导入到DevSSH配置中
2. 身份文件路径中的波浪号会被自动展开
3. 如果SSH配置文件不存在或无法读取，将回退到传统连接方式
4. 导入的主机可以在DevSSH的配置文件中查看和管理

## 向后兼容

所有现有功能保持不变：
- 仍然可以使用`devssh connect user@host:port`格式
- 命令行参数仍然有效
- 现有的DevSSH配置不受影响