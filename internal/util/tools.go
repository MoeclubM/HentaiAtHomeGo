// Package util 提供基础工具函数
package util

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// checkAndCreateDir 检查并创建目录
// 如果路径是文件则删除，如果目录不存在则创建
func CheckAndCreateDir(dir string) error {
	info, err := os.Stat(dir)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		// 如果是文件，删除它
		if err := os.Remove(dir); err != nil {
			return fmt.Errorf("无法删除文件 %s: %w", dir, err)
		}
	}

	// 创建目录
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录 %s: 检查权限和 I/O 错误", dir)
	}

	return nil
}

// GetStringFileContents 读取文本文件内容
func GetStringFileContents(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PutStringFileContents 写入文本文件内容
func PutStringFileContents(file, content string) error {
	return os.WriteFile(file, []byte(content), 0644)
}

// PutStringFileContentsWithCharset 写入文本文件内容（指定字符集）
// Go 默认使用 UTF-8，这里简化处理
func PutStringFileContentsWithCharset(file, content, charset string) error {
	return os.WriteFile(file, []byte(content), 0644)
}

// ListSortedFiles 返回目录中排序后的文件列表
func ListSortedFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		files = append(files, entry.Name())
	}

	sort.Strings(files)
	return files, nil
}

// ParseAdditional 解析 additional 参数（格式：key1=value1;key2=value2）
func ParseAdditional(additional string) map[string]string {
	result := make(map[string]string)

	if additional == "" || strings.TrimSpace(additional) == "" {
		return result
	}

	keyValuePairs := strings.Split(strings.TrimSpace(additional), ";")

	for _, kvPair := range keyValuePairs {
		// kvPair 至少需要3个字符才能是 k=v
		if len(kvPair) <= 2 {
			continue
		}

		parts := strings.SplitN(strings.TrimSpace(kvPair), "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" {
				result[key] = value
			}
		}
	}

	return result
}

// GetSHA1String 计算字符串的 SHA-1 哈希
func GetSHA1String(s string) string {
	hash := sha1.Sum([]byte(s))
	return strings.ToLower(hex.EncodeToString(hash[:]))
}

// GetSHA1File 计算文件的 SHA-1 哈希
func GetSHA1File(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	hasher := sha1.New()
	buffer := make([]byte, 65536) // 64KB 缓冲区

	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("读取文件失败: %w", err)
		}
		if n == 0 {
			break
		}
		hasher.Write(buffer[:n])
	}

	return strings.ToLower(hex.EncodeToString(hasher.Sum(nil))), nil
}

// BinaryToHex 将字节数组转换为十六进制字符串
func BinaryToHex(data []byte) string {
	return strings.ToLower(hex.EncodeToString(data))
}

// FileExists 检查文件是否存在
func FileExists(file string) bool {
	info, err := os.Stat(file)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists 检查目录是否存在
func DirExists(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetFileSize 获取文件大小
func GetFileSize(file string) (int64, error) {
	info, err := os.Stat(file)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// JoinPath 拼接路径
func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// MinInt 返回两数最小值
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MaxInt 返回两数最大值
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// RandomInt 返回 [0,max) 的加密随机整数
func RandomInt(max int) int {
	if max <= 1 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
