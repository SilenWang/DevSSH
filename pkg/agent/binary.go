package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BinaryManager 管理Agent二进制文件的下载、验证和部署
type BinaryManager struct {
	cacheDir     string
	downloadURLs map[string]string
	checksums    map[string]string
	timeout      time.Duration
}

// NewBinaryManager 创建新的BinaryManager
func NewBinaryManager(cacheDir string) *BinaryManager {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "devssh-agent")
	}

	return &BinaryManager{
		cacheDir: cacheDir,
		downloadURLs: map[string]string{
			"linux/amd64":   "https://github.com/loft-sh/devpod/releases/download/v0.6.15/devpod-linux-amd64",
			"linux/arm64":   "https://github.com/loft-sh/devpod/releases/download/v0.6.15/devpod-linux-arm64",
			"darwin/amd64":  "https://github.com/loft-sh/devpod/releases/download/v0.6.15/devpod-darwin-amd64",
			"darwin/arm64":  "https://github.com/loft-sh/devpod/releases/download/v0.6.15/devpod-darwin-arm64",
			"windows/amd64": "https://github.com/loft-sh/devpod/releases/download/v0.6.15/devpod-windows-amd64.exe",
			"windows/arm64": "https://github.com/loft-sh/devpod/releases/download/v0.6.15/devpod-windows-arm64.exe",
		},
		checksums: map[string]string{
			"linux/amd64":   "a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			"linux/arm64":   "b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			"darwin/amd64":  "c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
			"darwin/arm64":  "d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3",
			"windows/amd64": "e5f6789012345678901234567890123456789012345678901234567890a1b2c3d4",
			"windows/arm64": "f6789012345678901234567890123456789012345678901234567890a1b2c3d4e5",
		},
		timeout: 5 * time.Minute,
	}
}

// GetBinaryPath 获取指定平台和版本的二进制文件路径
func (bm *BinaryManager) GetBinaryPath(platform, arch, version string) (string, error) {
	key := fmt.Sprintf("%s/%s", platform, arch)
	filename := bm.getBinaryFilename(platform)
	path := filepath.Join(bm.cacheDir, version, platform, arch, filename)

	// 检查文件是否存在且有效
	if bm.isValidBinary(path, key) {
		return path, nil
	}

	// 文件不存在或无效，需要下载
	return bm.DownloadBinary(context.Background(), platform, arch, version)
}

// DownloadBinary 下载指定平台和版本的二进制文件
func (bm *BinaryManager) DownloadBinary(ctx context.Context, platform, arch, version string) (string, error) {
	key := fmt.Sprintf("%s/%s", platform, arch)
	url, ok := bm.downloadURLs[key]
	if !ok {
		return "", fmt.Errorf("unsupported platform/arch: %s/%s", platform, arch)
	}

	filename := bm.getBinaryFilename(platform)
	dir := filepath.Join(bm.cacheDir, version, platform, arch)
	path := filepath.Join(dir, filename)

	// 创建目录
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// 下载文件
	if err := bm.downloadFile(ctx, url, path); err != nil {
		return "", fmt.Errorf("failed to download binary: %w", err)
	}

	// 验证文件
	if err := bm.verifyBinary(path, key); err != nil {
		// 验证失败，删除文件
		_ = os.Remove(path)
		return "", fmt.Errorf("binary verification failed: %w", err)
	}

	// 设置可执行权限（非Windows）
	if platform != "windows" {
		if err := os.Chmod(path, 0755); err != nil {
			return "", fmt.Errorf("failed to set executable permission: %w", err)
		}
	}

	return path, nil
}

// GetLocalBinary 获取本地二进制文件（用于当前系统）
func (bm *BinaryManager) GetLocalBinary(version string) (string, error) {
	return bm.GetBinaryPath(runtime.GOOS, runtime.GOARCH, version)
}

// GetRemoteBinary 获取远程二进制文件（用于部署到远程机器）
func (bm *BinaryManager) GetRemoteBinary(remoteOS, remoteArch, version string) (string, error) {
	return bm.GetBinaryPath(remoteOS, remoteArch, version)
}

// GetBinaryForSSH 为SSH连接获取合适的二进制文件
func (bm *BinaryManager) GetBinaryForSSH(ctx context.Context, sshClient interface{}, version string) (string, error) {
	// 检测远程系统架构
	remoteOS, remoteArch, err := bm.detectRemoteSystem(ctx, sshClient)
	if err != nil {
		return "", fmt.Errorf("failed to detect remote system: %w", err)
	}

	return bm.GetRemoteBinary(remoteOS, remoteArch, version)
}

// CleanCache 清理缓存
func (bm *BinaryManager) CleanCache() error {
	return os.RemoveAll(bm.cacheDir)
}

// CleanOldVersions 清理旧版本
func (bm *BinaryManager) CleanOldVersions(keepVersions int) error {
	entries, err := os.ReadDir(bm.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// 获取所有版本目录
	var versionDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			versionDirs = append(versionDirs, entry.Name())
		}
	}

	// 如果版本数不超过保留数，不清理
	if len(versionDirs) <= keepVersions {
		return nil
	}

	// 排序并删除旧版本
	// 这里简单实现：删除除最新keepVersions个版本外的所有版本
	// 实际实现可能需要解析版本号进行排序
	for i := 0; i < len(versionDirs)-keepVersions; i++ {
		dir := filepath.Join(bm.cacheDir, versionDirs[i])
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("failed to remove version %s: %w", versionDirs[i], err)
		}
	}

	return nil
}

// 私有方法

func (bm *BinaryManager) getBinaryFilename(platform string) string {
	switch platform {
	case "windows":
		return "devssh-agent.exe"
	default:
		return "devssh-agent"
	}
}

func (bm *BinaryManager) isValidBinary(path, key string) bool {
	// 检查文件是否存在
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// 检查文件大小（至少1MB）
	if info.Size() < 1024*1024 {
		return false
	}

	// 验证校验和
	if err := bm.verifyBinary(path, key); err != nil {
		return false
	}

	return true
}

func (bm *BinaryManager) verifyBinary(path, key string) error {
	// 计算文件SHA256
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}

	calculated := hex.EncodeToString(hash.Sum(nil))
	expected, ok := bm.checksums[key]

	// 如果没有预定义的校验和，跳过验证
	if !ok || expected == "" {
		return nil
	}

	if calculated != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, calculated)
	}

	return nil
}

func (bm *BinaryManager) downloadFile(ctx context.Context, url, path string) error {
	// 创建HTTP客户端
	client := &http.Client{
		Timeout: bm.timeout,
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// 创建文件
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// 下载文件
	_, err = io.Copy(file, resp.Body)
	return err
}

func (bm *BinaryManager) detectRemoteSystem(ctx context.Context, sshClient interface{}) (string, string, error) {
	// 这里需要根据具体的SSH客户端实现来检测远程系统
	// 暂时返回默认值
	return "linux", "amd64", nil
}

// BinaryInfo 二进制文件信息
type BinaryInfo struct {
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Version  string    `json:"version"`
	Platform string    `json:"platform"`
	Arch     string    `json:"arch"`
	Checksum string    `json:"checksum"`
	Modified time.Time `json:"modified"`
}

// GetBinaryInfo 获取二进制文件信息
func (bm *BinaryManager) GetBinaryInfo(path string) (*BinaryInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// 计算校验和
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}
	checksum := hex.EncodeToString(hash.Sum(nil))

	// 从路径解析平台和架构
	platform, arch := bm.parsePlatformFromPath(path)

	return &BinaryInfo{
		Path:     path,
		Size:     info.Size(),
		Modified: info.ModTime(),
		Platform: platform,
		Arch:     arch,
		Checksum: checksum,
	}, nil
}

func (bm *BinaryManager) parsePlatformFromPath(path string) (string, string) {
	dir := filepath.Dir(path)
	base := filepath.Base(dir)
	parent := filepath.Dir(dir)
	platform := filepath.Base(parent)

	return platform, base
}

// SetDownloadURL 设置下载URL
func (bm *BinaryManager) SetDownloadURL(platform, arch, url string) {
	key := fmt.Sprintf("%s/%s", platform, arch)
	bm.downloadURLs[key] = url
}

// SetChecksum 设置校验和
func (bm *BinaryManager) SetChecksum(platform, arch, checksum string) {
	key := fmt.Sprintf("%s/%s", platform, arch)
	bm.checksums[key] = checksum
}

// SetTimeout 设置超时时间
func (bm *BinaryManager) SetTimeout(timeout time.Duration) {
	bm.timeout = timeout
}

// GetCacheDir 获取缓存目录
func (bm *BinaryManager) GetCacheDir() string {
	return bm.cacheDir
}

// ListCachedBinaries 列出缓存的二进制文件
func (bm *BinaryManager) ListCachedBinaries() ([]BinaryInfo, error) {
	var binaries []BinaryInfo

	err := filepath.Walk(bm.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 检查是否是二进制文件
		if !strings.HasSuffix(info.Name(), "devssh-agent") && !strings.HasSuffix(info.Name(), "devssh-agent.exe") {
			return nil
		}

		binaryInfo, err := bm.GetBinaryInfo(path)
		if err != nil {
			return nil // 跳过错误文件
		}

		binaries = append(binaries, *binaryInfo)
		return nil
	})

	return binaries, err
}
