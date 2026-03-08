// Package util 提供文件验证功能
package util

import (
	"sync"
)

// FileValidator 文件验证器
// 使用热/冷缓存机制避免重复的文件读取
type FileValidator struct {
	hotCache  map[string]bool
	coldCache map[string]bool
	mu        sync.RWMutex
}

// NewFileValidator 创建新的文件验证器
func NewFileValidator() *FileValidator {
	return &FileValidator{
		hotCache:  make(map[string]bool),
		coldCache: make(map[string]bool),
	}
}

// ValidateFile 验证文件 SHA-1 哈希
func (fv *FileValidator) ValidateFile(filePath, expectedHash string) bool {
	// 检查热缓存
	fv.mu.RLock()
	if hot, exists := fv.hotCache[filePath]; exists && hot {
		fv.mu.RUnlock()
		return true
	}
	if cold, exists := fv.coldCache[filePath]; exists && !cold {
		fv.mu.RUnlock()
		return false
	}
	fv.mu.RUnlock()

	// 计算文件哈希
	hash, err := GetSHA1File(filePath)
	if err != nil {
		return false
	}

	valid := hash == expectedHash

	// 更新缓存
	fv.mu.Lock()
	if valid {
		// 热缓存最大 1000 条目
		if len(fv.hotCache) < 1000 {
			fv.hotCache[filePath] = true
		}
	} else {
		// 冷缓存最大 10000 条目
		if len(fv.coldCache) < 10000 {
			fv.coldCache[filePath] = false
		}
	}
	fv.mu.Unlock()

	return valid
}

// ClearCache 清空缓存
func (fv *FileValidator) ClearCache() {
	fv.mu.Lock()
	defer fv.mu.Unlock()

	fv.hotCache = make(map[string]bool)
	fv.coldCache = make(map[string]bool)
}

// GetCacheStats 获取缓存统计
func (fv *FileValidator) GetCacheStats() (hot, cold int) {
	fv.mu.RLock()
	defer fv.mu.RUnlock()

	return len(fv.hotCache), len(fv.coldCache)
}
