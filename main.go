package main

import (
	"context"
	"fmt"
	"os"

	"github.com/loft-sh/devpod/pkg/agent/tunnelserver"
	"github.com/loft-sh/devpod/pkg/config"
	"github.com/loft-sh/devpod/pkg/ide/openvscode"
)

func main() {
	// 准备函数参数
	extensions := []string{}
	settings := `{
		"workbench.colorTheme": "Solarized Dark"
	}`
	userName := "sylens"
	host := "192.168.8.102"
	port := "22"
	values := map[string]config.OptionValue{
		"security": {Value: "strict"},
		"debug":    {Value: "true"},
	}
	// create a grpc client
	tunnelClient, err := tunnelserver.NewTunnelClient(os.Stdin, os.Stdout, true, 0)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	debug := true
	logger := tunnelserver.NewTunnelLogger(ctx, tunnelClient, debug)

	openVSCode := openvscode.NewOpenVSCodeServer(extensions, settings, userName, host, port, values, logger)

	// install open vscode
	// 这里出的问题, 也就是并没有安装成功
	// 经测试, 命令并没有在远程执行, 而是在本地执行了
	err = openVSCode.Install()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	openVSCode.Start()
}
