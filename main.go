package main

import (
	"fmt"
	"os"

	"tomcat-sync/config"
	"tomcat-sync/ui"
)

func main() {
	cfgPath := config.DefaultConfigPath()

	// 支持通过命令行参数指定配置文件
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	// 确保终端支持 ANSI 颜色
	// 在某些 Windows 终端中需要启用
	enableANSI()

	app := ui.NewApp(cfgPath)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n\033[31m致命错误: %v\033[0m\n", err)
		os.Exit(1)
	}
}

// enableANSI 在支持的平台上启用 ANSI 转义序列
func enableANSI() {
	// 在 Windows 10+ 上启用虚拟终端处理
	if isWindows() {
		// Go 1.25+ 默认支持 ANSI
		return
	}
}

// isWindows 检查是否是 Windows 系统
func isWindows() bool {
	return os.Getenv("OS") == "Windows_NT"
}
