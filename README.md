# DevSSH

A CLI tool to quickly set up remote development tools over SSH.

## Overview

`DevSSH` connects to a remote host via SSH, installs and launches [openvscode-server](https://github.com/gitpod-io/openvscode-server), then forwards its access port back to your local machine. It also provides fast port‑forwarding for other services, letting you start coding or debugging on a remote machine in seconds—without needing to pre‑build containers or manage complex configurations.

## Why DevSSH?

`DevSSH` draws inspiration from projects like [**sshcode**](https://github.com/coder/sshcode) (archived) and [**devpod**](https://github.com/loft-sh/devpod), but aims to be simpler and more direct for SSH‑based remote development. 

Even on devices with limited resources—such as Chromebooks (memory is constrained) or Fydetab Duo (GPU acceleration is incomplete)—DevSSH enables productive development by running all workloads on a remote machine.

## Development Tools

This project was developed using [opencode](https://github.com/anomalyco/opencode) and [DeepSeek](https://www.deepseek.com/). These AI-assisted tools were used to generate code, accelerate debugging, and improve overall development efficiency.

## Building from Source

If you have Go 1.21+ installed:
```bash
git clone https://github.com/SilenWang/DevSSH.git
cd DevSSH
go build -o devssh cmd/devssh/main.go 
```

## Build conda package

The recipe for building a Conda package for DevSSH with `rattler-build` is already configured in the source code. Simply run pixi build to create the package, then install it globally with pixi global install.

## License

Mozilla Public License 2.0 