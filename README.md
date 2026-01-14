# DevSSH

A CLI tool to quickly set up a remote development tools over SSH.

## Overview

`DevSSH` connects to a remote host via SSH, installs and launches [openvscode-server](https://github.com/gitpod-io/openvscode-server), then forwards its access port back to your local machine. It also provides fast port‑forwarding for other services, letting you start coding or debugging on a remote machine in seconds—without needing to pre‑build containers or manage complex configurations.

## Why DevSSH?

`DevSSH` draws inspiration from projects like [**sshcode**](https://github.com/coder/sshcode) (arhieved) and [**devpod**](https://github.com/loft-sh/devpod), but aims to be simpler and more direct for SSH‑based remote development. 

Even on devices with limited resources—such as Chromebooks(memory is constrained) or Fydetab Duo(GPU acceleration is incomplete), DevSSH enables productive development by running all workloads on a remote machine.

## Building from Source

If you have Go 1.21+ installed:
```bash
git clone https://github.com/yourusername/devssh.git
cd devssh
go build -o devssh cmd/devssh/main.go 
```

## License

Mozilla Public License 2.0 
