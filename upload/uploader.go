package upload

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sshclient "remotesync/ssh"
)

// UploadResult 单个文件的上传结果
type UploadResult struct {
	RelPath    string
	Success    bool
	Error      string
	FileSize   int64
	Duration   time.Duration
	BackupPath string // 备份路径
}

// BatchResult 批量上传结果
type BatchResult struct {
	Results      []UploadResult
	TotalSize    int64
	UploadedSize int64
	TotalTime    time.Duration
	SuccessCount int
	FailCount    int
}

// SuccessRate 返回成功率
func (r *BatchResult) SuccessRate() float64 {
	total := r.SuccessCount + r.FailCount
	if total == 0 {
		return 0
	}
	return float64(r.SuccessCount) / float64(total) * 100
}

// Uploader 文件上传器
type Uploader struct {
	sshClient  *sshclient.Client
	localRoot  string
	remoteRoot string
}

// NewUploader 创建上传器
func NewUploader(sshClient *sshclient.Client, localRoot, remoteRoot string) *Uploader {
	return &Uploader{
		sshClient:  sshClient,
		localRoot:  filepath.Clean(localRoot),
		remoteRoot: strings.TrimRight(remoteRoot, "/"),
	}
}

// UploadFile 上传单个文件（带备份）
func (u *Uploader) UploadFile(relPath string, callback func(status string)) UploadResult {
	startTime := time.Now()

	result := UploadResult{
		RelPath: relPath,
	}

	localPath := filepath.Join(u.localRoot, filepath.FromSlash(relPath))
	remotePath := u.remoteRoot + "/" + strings.ReplaceAll(relPath, "\\", "/")

	// 检查本地文件
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("本地文件不存在: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	result.FileSize = fileInfo.Size()

	if callback != nil {
		callback(fmt.Sprintf("备份: %s", relPath))
	}

	// 备份远程文件
	bakPath := remotePath + ".bak"
	if err := u.sshClient.BackupFile(remotePath); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("备份失败: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	result.BackupPath = bakPath

	if callback != nil {
		callback(fmt.Sprintf("上传: %s (%s)", relPath, FormatSize(result.FileSize)))
	}

	// 上传文件（带有重试机制）
	maxRetries := 3
	var uploadErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			if callback != nil {
				callback(fmt.Sprintf("正在重试 (%d/%d): %s", attempt, maxRetries, relPath))
			}
			time.Sleep(time.Second * time.Duration(attempt)) // 退避重试
		}

		uploadErr = u.sshClient.UploadFileViaSCP(localPath, remotePath, func(uploaded, total int64) {
			if callback != nil && total > 0 {
				pct := float64(uploaded) / float64(total) * 100
				callback(fmt.Sprintf("上传中: %s %.1f%%", relPath, pct))
			}
		})

		if uploadErr == nil {
			break
		}
	}

	if uploadErr != nil {
		result.Success = false
		result.Error = fmt.Sprintf("上传失败 (已尝试 %d 次): %v", maxRetries, uploadErr)
		result.Duration = time.Since(startTime)
		return result
	}

	result.Success = true
	result.Duration = time.Since(startTime)
	return result
}

// UploadFiles 批量上传文件（并行）
func (u *Uploader) UploadFiles(relPaths []string, concurrency int, callback func(status string)) *BatchResult {
	batchResult := &BatchResult{
		TotalSize: 0,
	}

	// 计算总大小
	for _, rp := range relPaths {
		localPath := filepath.Join(u.localRoot, filepath.FromSlash(rp))
		if info, err := os.Stat(localPath); err == nil {
			batchResult.TotalSize += info.Size()
		}
	}

	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(relPaths) {
		concurrency = len(relPaths)
	}

	// 限制并发数（SSH 连接不适合太多并发）
	if concurrency > 10 {
		concurrency = 10
	}

	startTime := time.Now()

	// 使用带缓冲的 channel 控制并发
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, rp := range relPaths {
		wg.Add(1)
		go func(relPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			mu.Lock()
			if callback != nil {
				callback(fmt.Sprintf("开始处理: %s", relPath))
			}
			mu.Unlock()

			result := u.UploadFile(relPath, func(status string) {
				mu.Lock()
				if callback != nil {
					callback(status)
				}
				mu.Unlock()
			})

			mu.Lock()
			batchResult.Results = append(batchResult.Results, result)
			if result.Success {
				batchResult.SuccessCount++
				batchResult.UploadedSize += result.FileSize
			} else {
				batchResult.FailCount++
			}
			mu.Unlock()
		}(rp)
	}

	wg.Wait()
	batchResult.TotalTime = time.Since(startTime)

	return batchResult
}

// FormatSize 格式化文件大小
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
