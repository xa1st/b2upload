package util

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FindFiles 根据给定的路径或模式查找文件
// 支持文件名、文件夹名或通配符模式 (如 *.png)
func FindFiles(pathOrPattern string) ([]string, error) {
	if pathOrPattern == "" {
		return nil, fmt.Errorf("文件路径或模式不能为空")
	}

	// 检查是否是文件夹
	if stat, err := os.Stat(pathOrPattern); err == nil && stat.IsDir() {
		// 如果是文件夹，查找所有文件
		return filepath.Glob(filepath.Join(pathOrPattern, "*"))
	}

	// 按模式查找 (包括单个文件名)
	return filepath.Glob(pathOrPattern)
}

// GetFileExt 获取文件扩展名，不带点
func GetFileExt(filePath string) string {
	ext := filepath.Ext(filePath)
	if len(ext) > 1 {
		return ext[1:] // 移除 '.'
	}
	return ""
}

// GenerateRemotePath 生成 Backblaze B2 期望的远程文件路径
// 格式：[用户名]/[当前年份4位]/[当前月日]/[16位md5(文件名)].[扩展名]
func GenerateRemotePath(localFile, user string) (string, error) {
	// 计算文件的md5
	hash, error := CalculateFileMD5(localFile)
	if error != nil {
		return "", error
	}
	hashStr := fmt.Sprintf("%x", hash)[:16] // 取前16位
	// 取扩展名
	ext := "." + GetFileExt(localFile)
	// 获取当前时间信息
	now := time.Now()
	year := now.Format("2006")
	monthDay := now.Format("0102")
	// 组合路径：[用户名]/[年份]/[月日]/[md5].[扩展名]
	remotePath := filepath.Join(user, year, monthDay, hashStr+ext)
	// B2 要求路径分隔符为 /
	return strings.ReplaceAll(remotePath, "\\", "/"), nil
}

// CalculateFileMD5 计算文件的 MD5 值 (用于 B2 的 X-Bz-Content-Md5 校验)
func CalculateFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("无法打开文件进行MD5计算: %w", err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("计算文件MD5时出错: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
