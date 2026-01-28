package download

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/loft-sh/log"
)

type LocalDownloader struct {
	cacheDir string
	logger   log.Logger
}

func NewLocalDownloader(cacheDir string, logger log.Logger) *LocalDownloader {
	return &LocalDownloader{
		cacheDir: cacheDir,
		logger:   logger,
	}
}

func (d *LocalDownloader) Download(url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("download URL is empty")
	}

	cachePath, err := d.getCachePath(url)
	if err != nil {
		return "", fmt.Errorf("failed to get cache path: %w", err)
	}

	if d.isCacheValid(cachePath) {
		d.logger.Debugf("Using cached file: %s", cachePath)
		return cachePath, nil
	}

	d.logger.Infof("Downloading...")

	if err := d.downloadFile(url, cachePath); err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}

	d.logger.Infof("Download complete: %s", filepath.Base(cachePath))
	return cachePath, nil
}

func (d *LocalDownloader) DownloadVSCode(version string) (string, error) {
	if version == "" {
		version = "v1.105.1"
	}

	url := d.getVSCodeDownloadURL(version)
	return d.Download(url)
}

func (d *LocalDownloader) getVSCodeDownloadURL(version string) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	baseURL := fmt.Sprintf("https://github.com/gitpod-io/openvscode-server/releases/download/%s/openvscode-server-%s", version, version)

	switch os {
	case "linux":
		if arch == "amd64" {
			return baseURL + "-linux-x64.tar.gz"
		} else if arch == "arm64" {
			return baseURL + "-linux-arm64.tar.gz"
		}
	case "darwin":
		if arch == "amd64" {
			return baseURL + "-darwin-x64.tar.gz"
		} else if arch == "arm64" {
			return baseURL + "-darwin-arm64.tar.gz"
		}
	}

	return baseURL + fmt.Sprintf("-%s-%s.tar.gz", os, arch)
}

func (d *LocalDownloader) getCachePath(url string) (string, error) {
	if err := os.MkdirAll(d.cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	hash := sha256.Sum256([]byte(url))
	filename := fmt.Sprintf("%x.tar.gz", hash[:8])
	return filepath.Join(d.cacheDir, filename), nil
}

func (d *LocalDownloader) isCacheValid(cachePath string) bool {
	info, err := os.Stat(cachePath)
	if err != nil {
		return false
	}

	if info.Size() == 0 {
		return false
	}

	return time.Since(info.ModTime()) < 30*24*time.Hour
}

func (d *LocalDownloader) downloadFile(url, destPath string) error {
	tempPath := destPath + ".tmp"
	defer os.Remove(tempPath)

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status: %s", resp.Status)
	}

	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

func (d *LocalDownloader) CleanOldCache(days int) error {
	if days <= 0 {
		days = 30
	}

	cutoffTime := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	entries, err := os.ReadDir(d.cacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			cachePath := filepath.Join(d.cacheDir, entry.Name())
			if err := os.Remove(cachePath); err != nil {
				d.logger.Warnf("Failed to remove old cache file %s: %v", entry.Name(), err)
			} else {
				d.logger.Debugf("Removed old cache file: %s", entry.Name())
			}
		}
	}

	return nil
}
