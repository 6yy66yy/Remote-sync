package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	sshclient "remotesync/ssh"
	"remotesync/config"
	"remotesync/compare"
	"remotesync/upload"
)

const (
	resetColor   = "\033[0m"
	bold         = "\033[1m"
	dim          = "\033[2m"
	red          = "\033[38;5;196m"
	green        = "\033[38;5;40m"
	yellow       = "\033[38;5;214m"
	blue         = "\033[38;5;33m"
	magenta      = "\033[38;5;170m"
	cyan         = "\033[38;5;45m"
	gray         = "\033[38;5;244m"
	bgBlue       = "\033[48;5;33m\033[38;5;255m"
	successLabel = "\033[48;5;40m\033[38;5;255m DONE \033[0m"
	errorLabel   = "\033[48;5;196m\033[38;5;255m FAIL \033[0m"
)

// App 主应用
type App struct {
	cfg       *config.Config
	cfgPath   string
	reader    *bufio.Reader
}

// NewApp 创建应用实例
func NewApp(cfgPath string) *App {
	return &App{
		cfgPath: cfgPath,
		reader:  bufio.NewReader(os.Stdin),
	}
}

// Run 启动应用
func (a *App) Run() error {
	// 加载配置
	var err error
	a.cfg, err = config.Load(a.cfgPath)
	if err != nil {
		fmt.Printf("%s⚠ 配置加载失败，将创建新配置: %v%s\n", yellow, err, resetColor)
		a.cfg = &config.Config{Servers: []config.Server{}}
	}

	fmt.Printf("\n%s%s\n  RemoteSync 远程同步工具 v1.0\n  快速对比本地与服务器文件差异，实现高效同步\n%s\n",
		bold+bgBlue+"                              "+resetColor,
		bold+"                              ",
		"══════════════════════════════════════════")

	for {
		err := a.mainMenu()
		if err != nil {
			fmt.Printf("\n%s错误: %v%s\n", red, err, resetColor)
		}
	}
}

// mainMenu 主菜单
func (a *App) mainMenu() error {
	a.printHeader("主菜单", "Home")

	fmt.Printf("    %s 服务器状态: %s%d 个已配置%s\n\n", gray+"●"+resetColor, bold, len(a.cfg.Servers), resetColor)

	fmt.Println("    " + gray + "┌────────────────────────────────────────┐" + resetColor)
	options := []struct {
		key  string
		desc string
	}{
		{"1", "选择服务器并同步文件"},
		{"2", "管理服务器配置"},
		{"3", "管理路径组合 (Group)"},
		{"0", "退出程序"},
	}

	for _, opt := range options {
		fmt.Printf("    "+gray+"│"+resetColor+"  %s%s%s. %-34s  "+gray+"│"+resetColor+"\n", cyan, opt.key, resetColor, opt.desc)
	}
	fmt.Println("    " + gray + "└────────────────────────────────────────┘" + resetColor)

	choice := a.prompt("请选择操作", "1")

	switch choice {
	case "1":
		if len(a.cfg.Servers) == 0 {
			fmt.Printf("\n    %s暂无服务器配置，请先添加服务器。%s\n", yellow, resetColor)
			a.pressEnter()
			return nil
		}
		return a.selectServerFlow()
	case "2":
		return a.manageServers()
	case "3":
		if len(a.cfg.Servers) == 0 {
			fmt.Printf("\n    %s暂无服务器配置，请先添加服务器。%s\n", yellow, resetColor)
			a.pressEnter()
			return nil
		}
		return a.manageGroups()
	case "0", "q", "Q":
		fmt.Printf("\n    %s再见！%s\n", green, resetColor)
		os.Exit(0)
	default:
		return fmt.Errorf("无效选择")
	}
	return nil
}

// ──────────────────────── 服务器管理 ────────────────────────

// manageServers 服务器管理菜单
func (a *App) manageServers() error {
	for {
		a.printHeader("服务器管理", "Home", "Config", "Servers")

		if len(a.cfg.Servers) == 0 {
			fmt.Printf("    %s暂无服务器配置%s\n\n", gray, resetColor)
		} else {
			for i, s := range a.cfg.Servers {
				fmt.Printf("    %s%2d.%s %s%-18s%s %s%s%s\n",
					cyan, i+1, resetColor,
					bold, s.Name, resetColor,
					gray, s.Host, resetColor)
			}
			fmt.Println()
		}

		fmt.Println("    " + gray + "┌────────────────────────────────────────┐" + resetColor)
		fmt.Printf("    " + gray + "│" + resetColor + "  %sA%s. 添加新服务器                    " + gray + "│" + resetColor + "\n", cyan, resetColor)
		if len(a.cfg.Servers) > 0 {
			fmt.Printf("    " + gray + "│" + resetColor + "  %sE%s. 编辑服务器                      " + gray + "│" + resetColor + "\n", cyan, resetColor)
			fmt.Printf("    " + gray + "│" + resetColor + "  %sD%s. 删除服务器                      " + gray + "│" + resetColor + "\n", red, resetColor)
			fmt.Printf("    " + gray + "│" + resetColor + "  %sV%s. 测试连接                        " + gray + "│" + resetColor + "\n", yellow, resetColor)
		}
		fmt.Printf("    " + gray + "│" + resetColor + "  %sB%s. 返回                            " + gray + "│" + resetColor + "\n", gray, resetColor)
		fmt.Println("    " + gray + "└────────────────────────────────────────┘" + resetColor)

		choice := a.prompt("请选择操作", "b")

		switch choice {
		case "a", "A":
			if err := a.addServer(); err != nil {
				return err
			}
		case "d", "D":
			if len(a.cfg.Servers) > 0 {
				if err := a.deleteServer(); err != nil {
					return err
				}
			}
		case "e", "E":
			if len(a.cfg.Servers) > 0 {
				if err := a.editServer(); err != nil {
					return err
				}
			}
		case "v", "V":
			if len(a.cfg.Servers) > 0 {
				if err := a.testConnection(); err != nil {
					return err
				}
			}
		case "b", "B", "0":
			return nil
		}
	}
}

// addServer 添加服务器
func (a *App) addServer() error {
	a.printHeader("添加服务器")

	name := a.promptRequired("服务器名称", "")
	host := a.promptRequired("主机地址", "")
	port := a.promptInt("SSH 端口", "22")
	username := a.promptRequired("用户名", "root")

	fmt.Printf("\n  %s认证方式:%s\n", cyan, resetColor)
	fmt.Println("  1. 密码认证")
	fmt.Println("  2. 密钥文件认证")
	authChoice := a.prompt("选择认证方式", "1")

	var password, keyPath string
	if authChoice == "2" {
		keyPath = a.promptRequired("密钥文件路径", "")
	} else {
		password = a.promptRequired("密码", "")
	}

	concurrency := a.promptInt("并发上传限制 (推荐 2-5)", "2")

	server := config.Server{
		Name:        name,
		Host:        host,
		Port:        port,
		Username:    username,
		Password:    password,
		KeyPath:     keyPath,
		Groups:      []config.Group{},
		Concurrency: concurrency,
	}

	a.cfg.AddServer(server)
	if err := a.saveConfig(); err != nil {
		return err
	}

	fmt.Printf("\n  %s✓ 服务器 '%s' 添加成功！%s\n", green, name, resetColor)

	// 询问是否立即添加组合
	addGroup := a.promptYesNo("是否立即为此服务器添加路径组合？", true)
	if addGroup {
		return a.addGroupToServer(&a.cfg.Servers[len(a.cfg.Servers)-1])
	}

	a.pressEnter()
	return nil
}

// deleteServer 删除服务器
func (a *App) deleteServer() error {
	idx := a.promptServerIndex()
	if idx < 0 {
		return nil
	}

	server := &a.cfg.Servers[idx]
	fmt.Printf("\n  确认删除服务器 %s'%s'%s？(y/N): ", red, server.Name, resetColor)
	answer, _ := a.reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "y" || answer == "yes" {
		name := server.Name
		a.cfg.RemoveServer(name)
		if err := a.saveConfig(); err != nil {
			return err
		}
		fmt.Printf("\n  %s✓ 服务器 '%s' 已删除%s\n", green, name, resetColor)
		a.pressEnter()
	}
	return nil
}

// editServer 编辑服务器
func (a *App) editServer() error {
	idx := a.promptServerIndex()
	if idx < 0 {
		return nil
	}

	server := &a.cfg.Servers[idx]
	a.printHeader(fmt.Sprintf("编辑服务器: %s", server.Name))

	fmt.Printf("  当前主机地址 [%s]: ", server.Host)
	input, _ := a.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		server.Host = input
	}

	fmt.Printf("  当前端口 [%d]: ", server.Port)
	input, _ = a.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		if p, err := strconv.Atoi(input); err == nil {
			server.Port = p
		}
	}

	fmt.Printf("  当前用户名 [%s]: ", server.Username)
	input, _ = a.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		server.Username = input
	}

	fmt.Printf("  修改密码？(y/N): ")
	answer, _ := a.reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "y" || answer == "yes" {
		server.Password = a.promptRequired("新密码", "")
	}

	fmt.Printf("  当前密钥路径 [%s]: ", server.KeyPath)
	input, _ = a.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		server.KeyPath = input
	}

	currConcurrency := server.Concurrency
	if currConcurrency <= 0 {
		currConcurrency = 2
	}
	fmt.Printf("  并发上传限制 [%d]: ", currConcurrency)
	input, _ = a.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		if c, err := strconv.Atoi(input); err == nil {
			server.Concurrency = c
		}
	}

	if err := a.saveConfig(); err != nil {
		return err
	}
	fmt.Printf("\n  %s✓ 服务器 '%s' 已更新%s\n", green, server.Name, resetColor)
	a.pressEnter()
	return nil
}

// testConnection 测试服务器连接
func (a *App) testConnection() error {
	idx := a.promptServerIndex()
	if idx < 0 {
		return nil
	}

	server := a.cfg.Servers[idx]
	fmt.Printf("\n  %s正在连接 %s@%s:%d ...%s\n", cyan, server.Username, server.Host, server.Port, resetColor)

	client, err := sshclient.NewClient(server.Host, server.Port, server.Username, server.Password, server.KeyPath)
	if err != nil {
		fmt.Printf("  %s✗ 连接失败: %v%s\n", red, err, resetColor)
		a.pressEnter()
		return nil
	}
	defer client.Close()

	fmt.Printf("  %s✓ SSH 连接成功！%s\n", green, resetColor)
	a.pressEnter()
	return nil
}

// ──────────────────────── 组合管理 ────────────────────────

// manageGroups 组合管理
func (a *App) manageGroups() error {
	idx := a.promptServerIndex()
	if idx < 0 {
		return nil
	}

	server := &a.cfg.Servers[idx]

	for {
		a.printHeader(fmt.Sprintf("组合管理 - %s", server.Name))

		if len(server.Groups) == 0 {
			fmt.Printf("  %s暂无路径组合%s\n\n", gray, resetColor)
		} else {
			for i, g := range server.Groups {
				fmt.Printf("  %s%d%s. %s%-20s%s\n", cyan, i+1, resetColor, bold, g.Name, resetColor)
				fmt.Printf("     远程: %s%s%s\n", gray, g.RemotePath, resetColor)
				fmt.Printf("     本地: %s%s%s\n", gray, g.LocalPath, resetColor)
				fmt.Println()
			}
		}

		fmt.Println("  ┌──────────────────────────────────────┐")
		fmt.Println("  │  a. 添加新组合                       │")
		if len(server.Groups) > 0 {
			fmt.Println("  │  c. 从已有组合复制到其他服务器       │")
			fmt.Println("  │  d. 删除组合                       │")
			fmt.Println("  │  e. 编辑组合                       │")
		}
		fmt.Println("  │  b. 返回                             │")
		fmt.Println("  └──────────────────────────────────────┘")

		choice := a.prompt("请选择操作", "b")

		switch choice {
		case "a", "A":
			if err := a.addGroupToServer(server); err != nil {
				return err
			}
		case "c", "C":
			if len(server.Groups) > 0 {
				if err := a.copyGroupToServer(server); err != nil {
					return err
				}
			}
		case "d", "D":
			if len(server.Groups) > 0 {
				if err := a.deleteGroup(server); err != nil {
					return err
				}
			}
		case "e", "E":
			if len(server.Groups) > 0 {
				if err := a.editGroup(server); err != nil {
					return err
				}
			}
		case "b", "B", "0":
			return nil
		}
	}
}

// addGroupToServer 为服务器添加组合
func (a *App) addGroupToServer(server *config.Server) error {
	a.printHeader(fmt.Sprintf("添加组合 - %s", server.Name))

	name := a.promptRequired("组合名称", "")
	remotePath := a.promptRequired("远程目录路径", "/opt/tomcat/webapps/")
	localPath := a.promptRequired("本地目录路径", "")

	group := config.Group{
		Name:       name,
		RemotePath: remotePath,
		LocalPath:  localPath,
	}

	server.AddGroup(group)
	if err := a.saveConfig(); err != nil {
		return err
	}

	fmt.Printf("\n  %s✓ 组合 '%s' 添加成功！%s\n", green, name, resetColor)
	a.pressEnter()
	return nil
}

// copyGroupToServer 复制组合到其他服务器
func (a *App) copyGroupToServer(sourceServer *config.Server) error {
	a.printHeader("复制组合到其他服务器")

	// 选择源组合
	fmt.Println("  选择源组合:")
	for i, g := range sourceServer.Groups {
		fmt.Printf("  %s%d%s. %s%s%s (远程: %s)\n", cyan, i+1, resetColor, bold, g.Name, resetColor, g.RemotePath)
	}

	grpIdx := a.promptInt("选择组合编号", "1")
	if grpIdx < 1 || grpIdx > len(sourceServer.Groups) {
		fmt.Printf("  %s无效选择%s\n", red, resetColor)
		return nil
	}

	sourceGroup := sourceServer.Groups[grpIdx-1]

	// 选择目标服务器
	fmt.Println("\n  选择目标服务器:")
	for i, s := range a.cfg.Servers {
		fmt.Printf("  %s%d%s. %s%s%s\n", cyan, i+1, resetColor, bold, s.Name, resetColor)
	}

	srvIdx := a.promptInt("选择目标服务器编号", "1")
	if srvIdx < 1 || srvIdx > len(a.cfg.Servers) {
		fmt.Printf("  %s无效选择%s\n", red, resetColor)
		return nil
	}

	targetServer := &a.cfg.Servers[srvIdx-1]

	// 输入新名称和路径
	newName := a.prompt(fmt.Sprintf("新组合名称 [%s]", sourceGroup.Name), sourceGroup.Name)
	newRemotePath := a.prompt(fmt.Sprintf("远程目录路径 [%s]", sourceGroup.RemotePath), sourceGroup.RemotePath)
	newLocalPath := a.prompt(fmt.Sprintf("本地目录路径 [%s]", sourceGroup.LocalPath), sourceGroup.LocalPath)

	newGroup := config.Group{
		Name:       newName,
		RemotePath: newRemotePath,
		LocalPath:  newLocalPath,
	}

	targetServer.AddGroup(newGroup)
	if err := a.saveConfig(); err != nil {
		return err
	}

	fmt.Printf("\n  %s✓ 组合已复制到服务器 '%s'，新组合名称: '%s'%s\n",
		green, targetServer.Name, newName, resetColor)
	a.pressEnter()
	return nil
}

// deleteGroup 删除组合
func (a *App) deleteGroup(server *config.Server) error {
	idx := a.promptGroupIndex(server)
	if idx < 0 {
		return nil
	}

	group := server.Groups[idx]
	fmt.Printf("\n  确认删除组合 %s'%s'%s？(y/N): ", red, group.Name, resetColor)
	answer, _ := a.reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "y" || answer == "yes" {
		name := group.Name
		server.RemoveGroup(name)
		if err := a.saveConfig(); err != nil {
			return err
		}
		fmt.Printf("\n  %s✓ 组合 '%s' 已删除%s\n", green, name, resetColor)
		a.pressEnter()
	}
	return nil
}

// editGroup 编辑组合
func (a *App) editGroup(server *config.Server) error {
	idx := a.promptGroupIndex(server)
	if idx < 0 {
		return nil
	}

	group := &server.Groups[idx]
	a.printHeader(fmt.Sprintf("编辑组合: %s", group.Name))

	fmt.Printf("  当前远程路径 [%s]: ", group.RemotePath)
	input, _ := a.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		group.RemotePath = input
	}

	fmt.Printf("  当前本地路径 [%s]: ", group.LocalPath)
	input, _ = a.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		group.LocalPath = input
	}

	if err := a.saveConfig(); err != nil {
		return err
	}
	fmt.Printf("\n  %s✓ 组合 '%s' 已更新%s\n", green, group.Name, resetColor)
	a.pressEnter()
	return nil
}

// ──────────────────────── 同步流程 ────────────────────────

// selectServerFlow 选择服务器并同步
func (a *App) selectServerFlow() error {
	a.printHeader("选择服务器", "Home", "Select Server")

	for i, s := range a.cfg.Servers {
		fmt.Printf("    %s%2d.%s %s%-18s%s %s%s%s %s(%d 组合)%s\n",
			cyan, i+1, resetColor,
			bold, s.Name, resetColor,
			gray, s.Host, resetColor,
			dim, len(s.Groups), resetColor)
	}
	fmt.Printf("\n    %s 0.%s 返回\n", yellow, resetColor)
	fmt.Println()

	idx := a.promptInt("选择服务器", "1")
	if idx == 0 {
		return nil
	}
	if idx < 1 || idx > len(a.cfg.Servers) {
		return fmt.Errorf("无效选择")
	}

	server := a.cfg.Servers[idx-1]
	return a.selectGroupFlow(server)
}

// selectGroupFlow 选择组合并同步
func (a *App) selectGroupFlow(server config.Server) error {
	if len(server.Groups) == 0 {
		fmt.Printf("\n  %s该服务器暂无路径组合，请先添加。%s\n", yellow, resetColor)
		a.pressEnter()
		return nil
	}

	// 连接服务器 (Connection Reuse Start)
	fmt.Printf("\n    %s正在连接 %s@%s:%d ...%s\n", cyan, server.Username, server.Host, server.Port, resetColor)
	client, err := sshclient.NewClient(server.Host, server.Port, server.Username, server.Password, server.KeyPath)
	if err != nil {
		return fmt.Errorf("SSH连接失败: %w", err)
	}
	defer client.Close()
	fmt.Printf("    %s✓ 连接成功！%s\n", green, resetColor)

	for {
		a.printHeader(fmt.Sprintf("选择组合 - %s", server.Name), "Home", "Select Server", server.Name)

		for i, g := range server.Groups {
			fmt.Printf("    %s%2d.%s %s%-20s%s\n", cyan, i+1, resetColor, bold, g.Name, resetColor)
			fmt.Printf("        %s💻 本地 (Local) : %s%s\n", gray, g.LocalPath, resetColor)
			fmt.Printf("        %s☁️  远程 (Remote): %s%s\n", gray, g.RemotePath, resetColor)
			fmt.Println()
		}
		
		fmt.Printf("    %s%2d.%s %s[批量并行同步所有组合]%s\n", magenta, len(server.Groups)+1, resetColor, bold, resetColor)
		fmt.Printf("\n    %s 0.%s 返回\n", yellow, resetColor)
		fmt.Println()

		idx := a.promptInt("选择组合", "1")
		if idx == 0 {
			return nil
		}
		if idx == len(server.Groups)+1 {
			if err := a.syncAllGroupsWithClient(client, server); err != nil {
				return err
			}
		} else if idx >= 1 && idx <= len(server.Groups) {
			group := server.Groups[idx-1]
			if err := a.syncFlow(client, server, group); err != nil {
				fmt.Printf("\n    %s同步过程中出错: %v%s\n", red, err, resetColor)
			}
		} else {
			fmt.Printf("\n    %s无效选择%s\n", red, resetColor)
		}

		if !a.promptYesNo("\n    是否继续选择该服务器中的其他组合？", true) {
			break
		}
	}

	return nil
}

func (a *App) syncAllGroupsWithClient(client *sshclient.Client, server config.Server) error {
	a.printHeader(fmt.Sprintf("批量并行同步 - %s", server.Name), "Home", "Batch Sync", server.Name)
	fmt.Printf("  %s即将对比并同步 %d 个组合%s\n", yellow, len(server.Groups), resetColor)

	if !a.promptYesNo("\n    确认开始批量同步？", false) {
		fmt.Printf("    %s已取消%s\n", yellow, resetColor)
		a.pressEnter()
		return nil
	}

	fmt.Printf("\n    %s正在并行对比所有组合...%s\n", cyan, resetColor)

	type GroupResult struct {
		group config.Group
		res   *compare.CompareResult
		err   error
	}
	resultsChan := make(chan GroupResult, len(server.Groups))
	var wg sync.WaitGroup

	for _, g := range server.Groups {
		wg.Add(1)
		go func(group config.Group) {
			defer wg.Done()
			res, err := compare.Compare(group.LocalPath, group.RemotePath, client, nil)
			resultsChan <- GroupResult{group, res, err}
		}(g)
	}

	wg.Wait()
	close(resultsChan)

	var allUploadTasks []struct {
		group config.Group
		diffs []compare.FileDiff
	}
	totalFiles := 0

	for gr := range resultsChan {
		if gr.err != nil {
			fmt.Printf("    %s✗ 组合 '%s' 对比失败: %v%s\n", red, gr.group.Name, gr.err, resetColor)
			continue
		}
		uploadable := gr.res.UploadableDiffs()
		if len(uploadable) > 0 {
			allUploadTasks = append(allUploadTasks, struct {
				group config.Group
				diffs []compare.FileDiff
			}{gr.group, uploadable})
			totalFiles += len(uploadable)
			fmt.Printf("    %s✓ %s: %d 个差异%s\n", green, gr.group.Name, len(uploadable), resetColor)
		} else {
			fmt.Printf("    %s✓ %s: 已是最新%s\n", gray, gr.group.Name, resetColor)
		}
	}

	if totalFiles == 0 {
		fmt.Printf("\n    %s✓ 所有组合均已是最新，无需同步。%s\n", green, resetColor)
		a.pressEnter()
		return nil
	}

	fmt.Printf("\n    %s准备上传所有组合共 %d 个文件...%s\n", yellow, totalFiles, resetColor)

	// 统一执行上传
	successCount := 0
	failCount := 0
	startTime := time.Now()

	concurrency := server.Concurrency
	if concurrency <= 0 {
		concurrency = 2
	}

	for _, task := range allUploadTasks {
		fmt.Printf("\n    %s同步组合: %s%s\n", bold, task.group.Name, resetColor)
		uploader := upload.NewUploader(client, task.group.LocalPath, task.group.RemotePath)

		paths := make([]string, len(task.diffs))
		for i, d := range task.diffs {
			paths[i] = d.RelPath
		}

		batchResult := uploader.UploadFiles(paths, concurrency, func(status string) {
			fmt.Printf("      %s%s%s\n", gray, status, resetColor)
		})

		successCount += batchResult.SuccessCount
		failCount += batchResult.FailCount
	}

	fmt.Printf("\n    %s批量同步完成!%s\n", green, resetColor)
	fmt.Printf("    总成功: %s%d%s | 总失败: %s%d%s | 总耗时: %s%s%s\n",
		green, successCount, resetColor,
		red, failCount, resetColor,
		bold, compare.FormatDuration(time.Since(startTime)), resetColor)

	a.pressEnter()
	return nil
}

// syncFlow 同步主流程
func (a *App) syncFlow(client *sshclient.Client, server config.Server, group config.Group) error {
	// 检查本地目录
	if _, err := os.Stat(group.LocalPath); os.IsNotExist(err) {
		return fmt.Errorf("本地目录不存在: %s", group.LocalPath)
	}

	return a.compareAndUploadFlow(client, group, server)
}

func (a *App) compareAndUploadFlow(client *sshclient.Client, group config.Group, server config.Server) error {
	// 对比目录
	fmt.Printf("\n    %s正在对比目录...%s\n", cyan, resetColor)

	result, err := compare.Compare(group.LocalPath, group.RemotePath, client, nil)
	if err != nil {
		return fmt.Errorf("目录对比失败: %w", err)
	}

	stats := result.Stats()

	// 显示对比结果
	a.printCompareResult(result, stats)
	uploadable := result.UploadableDiffs()

	if len(result.Diffs) == 0 {
		fmt.Printf("\n    %s✓ 所有文件已是最新，无需更新！%s\n", green, resetColor)
	}

	remoteOnly := result.RemoteOnlyDiffs()

	for {
		fmt.Println()
		fmt.Println("    ┌────────────────────────────────────────┐")
		menuItems := make(map[string]string)
		
		if len(uploadable) > 0 {
			fmt.Printf("    │  1. 全部上传 (%d 个可上传文件)%s  │\n", len(uploadable), "")
			menuItems["1"] = "upload_all"
			fmt.Printf("    │  2. 选择性上传%s                   │\n", "")
			menuItems["2"] = "selective_upload"
		}

		nextIdx := 3
		if len(uploadable) == 0 {
			nextIdx = 1
		}

		fmt.Printf("    │  %d. 查询现有文件%-25s │\n", nextIdx, "")
		menuItems[strconv.Itoa(nextIdx)] = "show_files"
		nextIdx++

		fmt.Printf("    │  %d. 重新对比%-29s │\n", nextIdx, "")
		menuItems[strconv.Itoa(nextIdx)] = "re-compare"
		nextIdx++

		if len(remoteOnly) > 0 {
			fmt.Printf("    │  %s%d. 清理远程冗余文件 (%d 个)%s        │\n", magenta, nextIdx, len(remoteOnly), resetColor)
			menuItems[strconv.Itoa(nextIdx)] = "cleanup"
			nextIdx++
		}

		fmt.Println("    │  0. 返回                               │")
		fmt.Println("    └────────────────────────────────────────┘")

		defaultChoice := "0"
		if len(uploadable) > 0 {
			defaultChoice = "1"
		}
		choice := a.prompt("请选择操作", defaultChoice)

		if choice == "0" {
			return nil
		}

		action := menuItems[choice]
		switch action {
		case "upload_all":
			return a.doUpload(client, server, group, uploadable, true)
		case "selective_upload":
			return a.selectiveUpload(client, server, group, uploadable)
		case "show_files":
			if err := a.showExistingFiles(client, group); err != nil {
				return err
			}
		case "re-compare":
			return a.compareAndUploadFlow(client, group, server)
		case "cleanup":
			if err := a.doCleanup(client, group, remoteOnly); err != nil {
				return err
			}
			return a.compareAndUploadFlow(client, group, server) // 清理后重新对比
		default:
			fmt.Printf("\n  %s无效选择%s\n", red, resetColor)
		}
	}
}

// doCleanup 执行远程清理
func (a *App) doCleanup(client *sshclient.Client, group config.Group, diffs []compare.FileDiff) error {
	if len(diffs) == 0 {
		return nil
	}

	fmt.Printf("\n    %s即将清理 ☁️  远程 (Remote) %d 个冗余文件:%s\n", red, len(diffs), resetColor)
	for _, d := range diffs {
		fmt.Printf("        %s-%s %s\n", red, resetColor, d.RelPath)
	}

	if !a.promptYesNo("\n    确认清理？(此操作不可恢复)", false) {
		fmt.Printf("    %s已取消%s\n", yellow, resetColor)
		a.pressEnter()
		return nil
	}

	fmt.Printf("\n    %s正在清理...%s\n", cyan, resetColor)
	success := 0
	fail := 0

	for _, d := range diffs {
		remotePath := group.RemotePath + "/" + d.RelPath
		if err := client.DeleteFile(remotePath); err != nil {
			fmt.Printf("    %s✗ 删除失败: %s - %v%s\n", red, d.RelPath, err, resetColor)
			fail++
		} else {
			fmt.Printf("    %s✓ 已删除: %s%s\n", gray, d.RelPath, resetColor)
			success++
		}
	}

	fmt.Printf("\n    %s清理完成: 成功 %d, 失败 %d%s\n", green, success, fail, resetColor)
	a.pressEnter()
	return nil
}

func (a *App) showExistingFiles(client *sshclient.Client, group config.Group) error {
	fmt.Printf("\n    %s正在查询远程现有文件...%s\n", cyan, resetColor)

	files, err := client.WalkDir(group.RemotePath)
	if err != nil {
		return fmt.Errorf("查询远程文件失败: %w", err)
	}

	a.printHeader("远程现有文件", "Home", "Sync", "Files")
	fmt.Printf("    %s ☁️  远程 (Remote): %s%s%s\n", gray+"●"+resetColor, bold, group.RemotePath, resetColor)
	fmt.Printf("    %s 文件总数: %s%d%s 个\n", gray+"●"+resetColor, cyan, len(files), resetColor)

	if len(files) == 0 {
		fmt.Printf("\n    %s该目录下暂无文件%s\n", yellow, resetColor)
		a.pressEnter()
		return nil
	}

	fmt.Println()
	fmt.Printf("    %s%-4s %-58s %-12s %-20s%s\n",
		bold, "序号", "文件路径", "大小", "修改时间", resetColor)
	fmt.Printf("    %s%s%s\n", gray, "────────────────────────────────────────────────────────────────────────────────────", resetColor)

	maxShow := 200
	for i, file := range files {
		if i >= maxShow {
			fmt.Printf("    %s... 还有 %d 个文件未显示%s\n", gray, len(files)-maxShow, resetColor)
			break
		}

		relPath := file.RelPath
		if len(relPath) > 56 {
			relPath = "..." + relPath[len(relPath)-53:]
		}

		fmt.Printf("    %-4d %-58s %-12s %-20s\n",
			i+1,
			relPath,
			compare.FormatSize(file.Size),
			file.ModTime.Format("2006-01-02 15:04:05"))
	}

	a.pressEnter()
	return nil
}

// printCompareResult 打印对比结果
func (a *App) printCompareResult(result *compare.CompareResult, stats compare.DiffStats) {
	a.printHeader("对比结果", "Sync", "Compare")

	fmt.Printf("    %s 💻 本地 (Local) : %s%s%s\n", gray+"●"+resetColor, bold, result.RootLocal, resetColor)
	fmt.Printf("    %s ☁️  远程 (Remote): %s%s%s\n", gray+"●"+resetColor, bold, result.RootRemote, resetColor)
	fmt.Printf("    %s 耗时: %s%s%s\n", gray+"●"+resetColor, cyan, compare.FormatDuration(result.Duration), resetColor)

	fmt.Println()
	fmt.Printf("    " + gray + "┌" + strings.Repeat("─", 40) + "┐" + resetColor + "\n")
	fmt.Printf("    " + gray + "│" + resetColor + "  " + green + "新增文件: %-28d" + resetColor + gray + "│" + resetColor + "\n", stats.NewCount)
	fmt.Printf("    " + gray + "│" + resetColor + "  " + yellow + "修改文件: %-28d" + resetColor + gray + "│" + resetColor + "\n", stats.ModifiedCount)
	fmt.Printf("    " + gray + "│" + resetColor + "  " + magenta + "仅在远程: %-28d" + resetColor + gray + "│" + resetColor + "\n", stats.RemoteOnlyCount)
	fmt.Printf("    " + gray + "└" + strings.Repeat("─", 40) + "┘" + resetColor + "\n")

	if len(result.Diffs) == 0 {
		return
	}

	fmt.Println()
	// 表头
	fmt.Printf("    %s%-8s %-40s %-12s%s\n", bold+gray, "TYPE", "FILE PATH", "SIZE DIFF", resetColor)
	fmt.Println("    " + gray + "────────────────────────────────────────────────────────────────" + resetColor)

	maxShow := 100
	for i, d := range result.Diffs {
		if i >= maxShow {
			fmt.Printf("    %s... and %d more differences%s\n", gray, len(result.Diffs)-maxShow, resetColor)
			break
		}

		relPath := d.RelPath
		if len(relPath) > 38 {
			relPath = "..." + relPath[len(relPath)-35:]
		}

		typeStr := fmt.Sprintf("%s%s%s", d.DiffType.ColorCode(), d.DiffType.String(), resetColor)

		var sizeStr string
		if d.DiffType == compare.DiffNew {
			sizeStr = fmt.Sprintf("%s+ %s%s", green, compare.FormatSize(d.LocalSize), resetColor)
		} else if d.DiffType == compare.DiffModified {
			diff := d.LocalSize - d.RemoteSize
			if diff >= 0 {
				sizeStr = fmt.Sprintf("%s+ %s%s", yellow, compare.FormatSize(diff), resetColor)
			} else {
				sizeStr = fmt.Sprintf("%s- %s%s", yellow, compare.FormatSize(-diff), resetColor)
			}
		} else {
			sizeStr = fmt.Sprintf("%s%s%s", gray, compare.FormatSize(d.RemoteSize), resetColor)
		}

		fmt.Printf("    %-8s %-40s %-12s\n", typeStr, relPath, sizeStr)
	}
}

// doUpload 执行上传
func (a *App) doUpload(client *sshclient.Client, server config.Server, group config.Group, diffs []compare.FileDiff, all bool) error {
	if len(diffs) == 0 {
		fmt.Printf("\n    %s没有可上传的文件%s\n", yellow, resetColor)
		a.pressEnter()
		return nil
	}

	// 确认上传
	if !all {
		fmt.Printf("\n    %s即将上传 %d 个文件:%s\n", yellow, len(diffs), resetColor)
		for _, d := range diffs {
			fmt.Printf("        %s+%s %s\n", green, resetColor, d.RelPath)
		}

		if !a.promptYesNo("\n    确认上传？", false) {
			fmt.Printf("    %s已取消%s\n", yellow, resetColor)
			a.pressEnter()
			return nil
		}
	} else {
		fmt.Printf("\n    %s正在准备上传全部 %d 个文件...%s\n", yellow, len(diffs), resetColor)
	}

	// 执行上传
	uploader := upload.NewUploader(client, group.LocalPath, group.RemotePath)
	fmt.Printf("\n    %s开始上传...%s\n\n", cyan, resetColor)

	// 获取所有需要上传的相对路径
	paths := make([]string, len(diffs))
	for i, d := range diffs {
		paths[i] = d.RelPath
	}

	// 使用配置的并发数，默认 2
	concurrency := server.Concurrency
	if concurrency <= 0 {
		concurrency = 2
	}

	batchResult := uploader.UploadFiles(paths, concurrency, func(status string) {
		fmt.Printf("      %s%s%s\n", gray, status, resetColor)
	})

	// 显示结果
	fmt.Println()
	a.printUploadResult(batchResult)

	a.pressEnter()
	return nil
}

// selectiveUpload 选择性上传
func (a *App) selectiveUpload(client *sshclient.Client, server config.Server, group config.Group, diffs []compare.FileDiff) error {
	uploadable := make([]compare.FileDiff, 0)
	for _, d := range diffs {
		if d.DiffType == compare.DiffNew || d.DiffType == compare.DiffModified {
			uploadable = append(uploadable, d)
		}
	}

	if len(uploadable) == 0 {
		fmt.Printf("\n    %s没有可上传的文件%s\n", yellow, resetColor)
		a.pressEnter()
		return nil
	}

	fmt.Println()
	fmt.Printf("    %s选择性上传%s (输入序号，多个用逗号分隔，如: 1,3,5)", bold, resetColor)
	fmt.Println()

	for i, d := range uploadable {
		fmt.Printf("    %s%3d%s. %s%-40s%s %s\n",
			cyan, i+1, resetColor,
			d.DiffType.ColorCode()+d.DiffType.String()+resetColor+" ",
			d.RelPath, gray,
			compare.FormatSize(d.LocalSize))
	}

	fmt.Println()
	selection := a.prompt("选择要上传的文件 (all=全部)", "")

	if strings.ToLower(selection) == "all" {
		return a.doUpload(client, server, group, uploadable, true)
	}

	// 解析选择
	indices := parseNumberList(selection)
	var selected []compare.FileDiff
	for _, idx := range indices {
		if idx >= 1 && idx <= len(uploadable) {
			selected = append(selected, uploadable[idx-1])
		}
	}

	if len(selected) == 0 {
		fmt.Printf("    %s未选择任何文件%s\n", yellow, resetColor)
		a.pressEnter()
		return nil
	}

	return a.doUpload(client, server, group, selected, false)
}

// printUploadResult 打印上传结果
func (a *App) printUploadResult(result *upload.BatchResult) {
	a.printHeader("上传结果", "Sync", "Result")

	fmt.Printf("    %s 成功: %s%d%s 个\n", green+"✔"+resetColor, bold, result.SuccessCount, resetColor)
	fmt.Printf("    %s 失败: %s%d%s 个\n", red+"✘"+resetColor, bold, result.FailCount, resetColor)
	fmt.Printf("    %s 大小: %s%s%s\n", gray+"●"+resetColor, bold, upload.FormatSize(result.TotalSize), resetColor)
	fmt.Printf("    %s 耗时: %s%s%s\n", gray+"●"+resetColor, bold, compare.FormatDuration(result.TotalTime), resetColor)

	if result.FailCount > 0 {
		fmt.Println()
		fmt.Printf("    %s失败详情:%s\n", red, resetColor)
		fmt.Println("    " + gray + "────────────────────────────────────────────────────────────────" + resetColor)
		for _, r := range result.Results {
			if !r.Success {
				fmt.Printf("    %s %-30s %s%v%s\n", errorLabel, r.RelPath, red, r.Error, resetColor)
			}
		}
	} else {
		fmt.Println()
		fmt.Printf("    %s 所有文件已同步成功！%s\n", successLabel, resetColor)
	}
}

// ──────────────────────── 辅助方法 ────────────────────────

func (a *App) printHeader(title string, breadcrumbs ...string) {
	fmt.Print("\033[H\033[2J") // 清屏
	
	if len(breadcrumbs) > 0 {
		bc := strings.Join(breadcrumbs, " "+gray+"›"+resetColor+" ")
		fmt.Printf("  %s %s %s\n", gray+"Location:", bc, resetColor)
	}

	fmt.Println("  " + blue + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + resetColor)
	fmt.Printf("  %s %s\n", bold+blue+"▶"+resetColor, bold+title)
	fmt.Println("  " + blue + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + resetColor)
	fmt.Println()
}

func (a *App) prompt(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s%s%s [%s]: ", cyan, label, resetColor, defaultVal)
	} else {
		fmt.Printf("  %s%s%s: ", cyan, label, resetColor)
	}
	input, _ := a.reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

func (a *App) promptRequired(label, defaultVal string) string {
	for {
		val := a.prompt(label, defaultVal)
		if val != "" {
			return val
		}
		fmt.Printf("  %s此项为必填！%s\n", red, resetColor)
	}
}

func (a *App) promptInt(label, defaultVal string) int {
	val := a.prompt(label, defaultVal)
	n, _ := strconv.Atoi(val)
	return n
}

func (a *App) promptYesNo(label string, defaultYes bool) bool {
	suffix := "(y/N)"
	if defaultYes {
		suffix = "(Y/n)"
	}
	fmt.Printf("  %s%s%s %s: ", cyan, label, resetColor, suffix)
	answer, _ := a.reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes"
}

func (a *App) promptServerIndex() int {
	fmt.Println()
	for i, s := range a.cfg.Servers {
		fmt.Printf("  %s%d%s. %s%s%s (%s:%d)\n",
			cyan, i+1, resetColor, bold, s.Name, resetColor, s.Host, s.Port)
	}
	fmt.Printf("  %s0%s. 返回\n", cyan, resetColor)

	idx := a.promptInt("选择服务器", "0")
	if idx == 0 {
		return -1
	}
	if idx < 1 || idx > len(a.cfg.Servers) {
		fmt.Printf("  %s无效选择%s\n", red, resetColor)
		return -1
	}
	return idx - 1
}

func (a *App) promptGroupIndex(server *config.Server) int {
	fmt.Println()
	for i, g := range server.Groups {
		fmt.Printf("  %s%d%s. %s%s%s\n", cyan, i+1, resetColor, bold, g.Name, resetColor)
	}
	fmt.Printf("  %s0%s. 返回\n", cyan, resetColor)

	idx := a.promptInt("选择组合", "0")
	if idx == 0 {
		return -1
	}
	if idx < 1 || idx > len(server.Groups) {
		fmt.Printf("  %s无效选择%s\n", red, resetColor)
		return -1
	}
	return idx - 1
}

func (a *App) pressEnter() {
	fmt.Printf("\n  %s按回车键继续...%s ", gray, resetColor)
	a.reader.ReadString('\n')
}

func (a *App) saveConfig() error {
	if err := config.Save(a.cfgPath, a.cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	return nil
}

// parseNumberList 解析逗号分隔的数字列表
func parseNumberList(s string) []int {
	var result []int
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		n, err := strconv.Atoi(p)
		if err == nil && n > 0 {
			result = append(result, n)
		}
	}
	return result
}
