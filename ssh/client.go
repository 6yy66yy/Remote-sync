package ssh

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// FileInfo 远程文件信息
type FileInfo struct {
	Name     string
	Size     int64
	ModTime  time.Time
	IsDir    bool
	RelPath  string // 相对于根目录的相对路径
}

// Client SSH/SFTP 客户端
type Client struct {
	client  *ssh.Client
	host    string
	timeout time.Duration
}

// NewClient 创建新的 SSH 客户端连接
func NewClient(host string, port int, username, password, keyPath string) (*Client, error) {
	var authMethods []ssh.AuthMethod

	// 优先使用密钥认证
	if keyPath != "" {
		key, err := os.ReadFile(keyPath)
		if err == nil {
			signer, err := ssh.ParsePrivateKey(key)
			if err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}

	// 密码认证
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("未提供有效的认证方式（密码或密钥）")
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败 [%s]: %w", addr, err)
	}

	return &Client{
		client:  client,
		host:    host,
		timeout: 30 * time.Second,
	}, nil
}

// Close 关闭连接
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// executeCommand 执行远程命令
func (c *Client) executeCommand(cmd string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return string(output), err
}

// WalkDir 递归遍历远程目录，返回文件列表
func (c *Client) WalkDir(remotePath string) ([]FileInfo, error) {
	// 首先检查目录是否存在
	output, err := c.executeCommand(fmt.Sprintf("test -d %s && echo EXISTS", quotePath(remotePath)))
	if err != nil || strings.TrimSpace(output) != "EXISTS" {
		return nil, fmt.Errorf("远程目录不存在: %s", remotePath)
	}

	// 使用 find 命令获取所有文件信息。
	// 这里直接输出相对路径（%P），避免依赖字符串裁剪绝对路径前缀，
	// 否则当远程根目录带尾斜杠、软链接或路径格式变化时，可能导致所有文件都匹配失败。
	cmd := fmt.Sprintf(
		`find %s -type f -printf '%%P|%%s|%%T@\n' 2>/dev/null | head -50000`,
		quotePath(remotePath),
	)

	output, err = c.executeCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("执行远程find命令失败: %w", err)
	}

	var files []FileInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}

		relPath := normalizeRemoteRelPath(parts[0])
		if relPath == "" {
			continue
		}

		var size int64
		fmt.Sscanf(parts[1], "%d", &size)

		var modTimeSec float64
		fmt.Sscanf(parts[2], "%f", &modTimeSec)
		modTime := time.Unix(int64(modTimeSec), int64((modTimeSec-float64(int64(modTimeSec)))*1e9))

		files = append(files, FileInfo{
			Name:     path.Base(relPath),
			Size:     size,
			ModTime:  modTime.Local(),
			IsDir:    false,
			RelPath:  relPath,
		})
	}

	return files, nil
}

func normalizeRemoteRelPath(relPath string) string {
	relPath = strings.TrimSpace(relPath)
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	relPath = path.Clean("/" + relPath)
	relPath = strings.TrimPrefix(relPath, "/")
	if relPath == "." {
		return ""
	}
	return relPath
}

// Stat 获取远程文件/目录信息
func (c *Client) Stat(remotePath string) (*FileInfo, error) {
	cmd := fmt.Sprintf(
		`stat -c '%%n|%%s|%%Y|%%F' %s 2>/dev/null`,
		quotePath(remotePath),
	)
	output, err := c.executeCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("获取远程文件信息失败: %w", err)
	}

	parts := strings.SplitN(strings.TrimSpace(output), "|", 4)
	if len(parts) != 4 {
		return nil, fmt.Errorf("解析文件信息失败")
	}

	var size int64
	fmt.Sscanf(parts[1], "%d", &size)

	var modTimeSec int64
	fmt.Sscanf(parts[2], "%d", &modTimeSec)

	isDir := strings.TrimSpace(parts[3]) == "directory"

	return &FileInfo{
		Name:    filepath.Base(parts[0]),
		Size:    size,
		ModTime: time.Unix(modTimeSec, 0).Local(),
		IsDir:   isDir,
	}, nil
}

// EnsureDir 确保远程目录存在（递归创建）
func (c *Client) EnsureDir(remotePath string) error {
	cmd := fmt.Sprintf("mkdir -p %s", quotePath(remotePath))
	_, err := c.executeCommand(cmd)
	if err != nil {
		return fmt.Errorf("创建远程目录失败: %w", err)
	}
	return nil
}

// BackupFile 备份远程文件：将原文件重命名为 .bak，如果已有 .bak 则先删除旧备份
func (c *Client) BackupFile(remoteFilePath string) error {
	bakPath := remoteFilePath + ".bak"

	// 如果已存在 .bak 文件，先删除（仅保留本次备份）
	_, err := c.executeCommand(fmt.Sprintf("rm -f %s", quotePath(bakPath)))
	if err != nil {
		return fmt.Errorf("删除旧备份文件失败: %w", err)
	}

	// 检查原文件是否存在
	output, err := c.executeCommand(fmt.Sprintf("test -f %s && echo EXISTS", quotePath(remoteFilePath)))
	if err != nil || strings.TrimSpace(output) != "EXISTS" {
		// 文件不存在，无需备份
		return nil
	}

	// 重命名为 .bak
	_, err = c.executeCommand(fmt.Sprintf("mv %s %s", quotePath(remoteFilePath), quotePath(bakPath)))
	if err != nil {
		return fmt.Errorf("备份文件失败: %w", err)
	}

	return nil
}

// UploadFile 通过 SCP 方式上传本地文件到远程
func (c *Client) UploadFile(localPath, remotePath string, progress func(uploaded, total int64)) error {
	return c.UploadFileViaSCP(localPath, remotePath, progress)
}

// UploadFileViaSCP 通过 SCP 协议上传文件
func (c *Client) UploadFileViaSCP(localPath, remotePath string, progress func(uploaded, total int64)) error {
	// 获取本地文件的修改时间（用于上传后同步）
	localInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("获取本地文件信息失败: %w", err)
	}
	localModTime := localInfo.ModTime()

	// 确保远程目录存在
	remoteDir := path.Dir(remotePath)
	if err := c.EnsureDir(remoteDir); err != nil {
		return err
	}

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("创建SCP会话失败: %w", err)
	}
	defer session.Close()

	if progress == nil {
		progress = func(_, _ int64) {}
	}

	// 使用 SCP 协议传输
	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()

		// SCP 协议头: C<mode> <size> <filename>\n
		fmt.Fprintf(w, "C%04o %d %s\n", stat.Mode().Perm(), stat.Size(), filepath.Base(remotePath))

		var uploaded int64
		buf := make([]byte, 64*1024)
		for {
			n, readErr := f.Read(buf)
			if n > 0 {
				_, writeErr := w.Write(buf[:n])
				if writeErr != nil {
					break
				}
				uploaded += int64(n)
				progress(uploaded, stat.Size())
			}
			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				break
			}
		}
		fmt.Fprint(w, "\x00") // 传输完成标志
	}()

	err = session.Run(fmt.Sprintf("scp -t %s", quotePath(remotePath)))
	if err != nil {
		return fmt.Errorf("SCP传输失败: %w", err)
	}

	// 上传成功后，将远程文件的修改时间设置为与本地源文件一致
	if touchErr := c.SetModTime(remotePath, localModTime); touchErr != nil {
		return fmt.Errorf("设置文件修改时间失败: %w", touchErr)
	}

	return nil
}

// DeleteFile 删除远程文件
func (c *Client) DeleteFile(remotePath string) error {
	_, err := c.executeCommand(fmt.Sprintf("rm -f %s", quotePath(remotePath)))
	if err != nil {
		return fmt.Errorf("删除远程文件失败: %w", err)
	}
	return nil
}

// MakeDir 创建远程目录
func (c *Client) MakeDir(remotePath string) error {
	return c.EnsureDir(remotePath)
}

// SetModTime 设置远程文件的修改时间
func (c *Client) SetModTime(remotePath string, modTime time.Time) error {
	// 使用 touch 命令设置文件的修改时间和访问时间
	// 格式: touch -t YYYYMMDDhhmm.ss filepath
	timeStr := modTime.Format("200601021504.05")
	cmd := fmt.Sprintf("touch -t %s %s", timeStr, quotePath(remotePath))
	_, err := c.executeCommand(cmd)
	if err != nil {
		return fmt.Errorf("设置修改时间失败: %w", err)
	}
	return nil
}

// CalculateRemoteSHA256 计算远程文件的 SHA256。
// 优先使用 sha256sum，兼容性不足时回退到 shasum -a 256。
func (c *Client) CalculateRemoteSHA256(remotePath string) (string, error) {
	cmd := fmt.Sprintf(
		"(sha256sum %s 2>/dev/null || shasum -a 256 %s 2>/dev/null) | awk '{print $1}'",
		quotePath(remotePath),
		quotePath(remotePath),
	)
	output, err := c.executeCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("执行远程SHA256命令失败: %w", err)
	}

	hash := strings.TrimSpace(output)
	if hash == "" {
		return "", fmt.Errorf("远程SHA256结果为空")
	}
	return hash, nil
}

// quotePath 对路径进行安全引用
func quotePath(path string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(path, "'", "'\\''"))
}
