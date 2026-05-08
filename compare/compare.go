package compare

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	sshclient "remotesync/ssh"
)

var defaultIgnorePatterns = []string{
	"*.bak",
}

// DiffType 差异类型
type DiffType int

const (
	DiffNew       DiffType = iota // 仅本地存在（新文件）
	DiffModified                   // 文件已修改（更新时间不同）
	DiffRemoteOnly                 // 仅远程存在（服务器上有，本地无）
)

func (d DiffType) String() string {
	switch d {
	case DiffNew:
		return "新增"
	case DiffModified:
		return "更新"
	case DiffRemoteOnly:
		return "仅远程"
	default:
		return "未知"
	}
}

func (d DiffType) ColorCode() string {
	switch d {
	case DiffNew:
		return "\033[32m" // 绿色
	case DiffModified:
		return "\033[33m" // 黄色
	case DiffRemoteOnly:
		return "\033[35m" // 紫色
	default:
		return "\033[0m"
	}
}

// FileDiff 单个文件的差异信息
type FileDiff struct {
	RelPath      string   // 相对路径
	DiffType     DiffType // 差异类型
	LocalSize    int64    // 本地文件大小
	RemoteSize   int64    // 远程文件大小
	LocalModTime time.Time
	RemoteModTime time.Time
}

// CompareResult 对比结果
type CompareResult struct {
	RootLocal  string     // 本地根路径
	RootRemote string     // 远程根路径
	Diffs      []FileDiff // 差异列表
	TotalLocal int        // 本地文件总数
	TotalRemote int       // 远程文件总数
	StartTime  time.Time  // 对比开始时间
	Duration   time.Duration // 对比耗时
}

// DiffStats 差异统计
type DiffStats struct {
	NewCount        int
	ModifiedCount   int
	RemoteOnlyCount int
}

// Stats 返回统计信息
func (r *CompareResult) Stats() DiffStats {
	stats := DiffStats{}
	for _, d := range r.Diffs {
		switch d.DiffType {
		case DiffNew:
			stats.NewCount++
		case DiffModified:
			stats.ModifiedCount++
		case DiffRemoteOnly:
			stats.RemoteOnlyCount++
		}
	}
	return stats
}

// FilterByType 按差异类型过滤
func (r *CompareResult) FilterByType(diffType DiffType) []FileDiff {
	var result []FileDiff
	for _, d := range r.Diffs {
		if d.DiffType == diffType {
			result = append(result, d)
		}
	}
	return result
}

// UploadableDiffs 返回可上传的差异（新增+修改）
func (r *CompareResult) UploadableDiffs() []FileDiff {
	var result []FileDiff
	for _, d := range r.Diffs {
		if d.DiffType == DiffNew || d.DiffType == DiffModified {
			result = append(result, d)
		}
	}
	return result
}

// RemoteOnlyDiffs 返回仅远程存在的差异（用于清理）
func (r *CompareResult) RemoteOnlyDiffs() []FileDiff {
	var result []FileDiff
	for _, d := range r.Diffs {
		if d.DiffType == DiffRemoteOnly {
			result = append(result, d)
		}
	}
	return result
}

// Compare 对比本地目录和远程目录
func Compare(localPath, remotePath string, sshClient *sshclient.Client, ignorePatterns []string) (*CompareResult, error) {
	startTime := time.Now()
	ignorePatterns = mergeIgnorePatterns(ignorePatterns, defaultIgnorePatterns)

	// 标准化路径
	localPath = filepath.Clean(localPath)
	remotePath = strings.TrimRight(remotePath, "/")

	// 并行获取本地和远程文件列表
	var localFiles []localFileInfo
	var remoteFiles []sshclient.FileInfo
	var localErr, remoteErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		localFiles, localErr = walkLocalDir(localPath)
	}()

	go func() {
		defer wg.Done()
		remoteFiles, remoteErr = sshClient.WalkDir(remotePath)
	}()

	wg.Wait()

	if localErr != nil {
		return nil, fmt.Errorf("遍历本地目录失败: %w", localErr)
	}
	if remoteErr != nil {
		return nil, fmt.Errorf("遍历远程目录失败: %w", remoteErr)
	}

	// 构建文件映射
	localMap := make(map[string]*localFileInfo)
	for i := range localFiles {
		normalized := normalizeRelPath(localFiles[i].relPath)
		localFiles[i].relPath = normalized
		localMap[normalized] = &localFiles[i]
	}

	remoteMap := make(map[string]*sshclient.FileInfo)
	for i := range remoteFiles {
		normalized := normalizeRelPath(remoteFiles[i].RelPath)
		remoteFiles[i].RelPath = normalized
		remoteMap[normalized] = &remoteFiles[i]
	}

	// 对比差异
	var diffs []FileDiff

	// 检查本地文件
	for relPath, localInfo := range localMap {
		if shouldIgnore(relPath, ignorePatterns) {
			continue
		}

		remoteInfo, exists := remoteMap[relPath]
		if !exists {
			// 仅本地存在 → 新增
			diffs = append(diffs, FileDiff{
				RelPath:       relPath,
				DiffType:      DiffNew,
				LocalSize:     localInfo.size,
				LocalModTime:  localInfo.modTime,
			})
		} else {
			modified, err := isFileModified(localInfo, remoteInfo, remotePath, sshClient)
			if err != nil {
				return nil, fmt.Errorf("对比文件失败 [%s]: %w", relPath, err)
			}
			if modified {
				diffs = append(diffs, FileDiff{
					RelPath:       relPath,
					DiffType:      DiffModified,
					LocalSize:     localInfo.size,
					RemoteSize:    remoteInfo.Size,
					LocalModTime:  localInfo.modTime,
					RemoteModTime: remoteInfo.ModTime,
				})
			}
		}
	}

	// 检查远程独有文件
	for relPath, remoteInfo := range remoteMap {
		if shouldIgnore(relPath, ignorePatterns) {
			continue
		}
		if _, exists := localMap[relPath]; !exists {
			diffs = append(diffs, FileDiff{
				RelPath:       relPath,
				DiffType:      DiffRemoteOnly,
				RemoteSize:    remoteInfo.Size,
				RemoteModTime: remoteInfo.ModTime,
			})
		}
	}

	// 排序：按差异类型优先级，然后按路径
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].DiffType != diffs[j].DiffType {
			return diffs[i].DiffType < diffs[j].DiffType
		}
		return diffs[i].RelPath < diffs[j].RelPath
	})

	return &CompareResult{
		RootLocal:   localPath,
		RootRemote:  remotePath,
		Diffs:       diffs,
		TotalLocal:  len(localFiles),
		TotalRemote: len(remoteFiles),
		StartTime:   startTime,
		Duration:    time.Since(startTime),
	}, nil
}

type localFileInfo struct {
	relPath  string
	size     int64
	modTime  time.Time
	absPath  string
}

// walkLocalDir 递归遍历本地目录
func walkLocalDir(root string) ([]localFileInfo, error) {
	var files []localFileInfo

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// 跳过无法访问的文件
			return nil
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		// 统一路径分隔符为正斜杠
		relPath = normalizeRelPath(filepath.ToSlash(relPath))

		files = append(files, localFileInfo{
			relPath: relPath,
			size:    info.Size(),
			modTime: info.ModTime(),
			absPath: path,
		})

		return nil
	})

	return files, err
}

func isFileModified(localInfo *localFileInfo, remoteInfo *sshclient.FileInfo, remoteRoot string, sshClient *sshclient.Client) (bool, error) {
	if localInfo.size != remoteInfo.Size {
		return true, nil
	}

	timeDiff := localInfo.modTime.Sub(remoteInfo.ModTime)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	if timeDiff <= time.Second {
		return false, nil
	}

	localHash, err := calculateLocalSHA256(localInfo.absPath)
	if err != nil {
		return false, fmt.Errorf("计算本地SHA256失败: %w", err)
	}

	remoteHash, err := sshClient.CalculateRemoteSHA256(buildRemoteFilePath(remoteRoot, localInfo.relPath))
	if err != nil {
		return false, fmt.Errorf("计算远程SHA256失败: %w", err)
	}

	return !strings.EqualFold(localHash, remoteHash), nil
}

func calculateLocalSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func buildRemoteFilePath(remoteRoot, relPath string) string {
	remoteRoot = strings.TrimRight(remoteRoot, "/")
	relPath = strings.TrimLeft(relPath, "/")
	if relPath == "" {
		return remoteRoot
	}
	return remoteRoot + "/" + relPath
}

func normalizeRelPath(relPath string) string {
	relPath = strings.TrimSpace(relPath)
	relPath = filepath.ToSlash(relPath)
	relPath = path.Clean("/" + relPath)
	relPath = strings.TrimPrefix(relPath, "/")
	if relPath == "." {
		return ""
	}
	return relPath
}

// shouldIgnore 检查文件是否应被忽略
func shouldIgnore(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, relPath)
		if err == nil && matched {
			return true
		}
		// 也检查是否以 pattern 作为目录匹配
		if strings.HasPrefix(relPath, pattern+"/") {
			return true
		}
	}
	return false
}

func mergeIgnorePatterns(patterns []string, defaults []string) []string {
	merged := make([]string, 0, len(defaults)+len(patterns))
	merged = append(merged, defaults...)
	merged = append(merged, patterns...)
	return merged
}

// FormatDuration 格式化耗时
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
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
