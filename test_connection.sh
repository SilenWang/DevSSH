#!/bin/bash

echo "测试 DevSSH 连接..."
echo "注意：这只是一个模拟测试，不会实际连接远程服务器"
echo ""

echo "1. 显示帮助信息："
./devssh --help
echo ""

echo "2. 显示 connect 命令帮助："
./devssh connect --help
echo ""

echo "3. 显示 install 命令帮助："
./devssh install --help
echo ""

echo "4. 显示 forward 命令帮助："
./devssh forward --help
echo ""

echo "5. 测试构建是否成功："
if [ -f "./devssh" ]; then
    echo "✓ 构建成功：devssh 可执行文件存在"
    file ./devssh
else
    echo "✗ 构建失败：devssh 可执行文件不存在"
fi
echo ""

echo "6. 测试版本信息："
./devssh --version
echo ""

echo "测试完成！"
echo ""
echo "实际使用示例："
echo "  ./devssh install user@example.com --ide vscode"
echo "  ./devssh connect user@example.com --ide vscode --forward 3000,8080:80"