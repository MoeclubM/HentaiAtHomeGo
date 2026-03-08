// Package cache 提供缓存管理功能
package cache

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/network"
	"github.com/qwq/hentaiathomego/internal/stats"
	"github.com/qwq/hentaiathomego/internal/util"
	"github.com/qwq/hentaiathomego/pkg/hvfile"
)

const (
	LRU_CACHE_SIZE = 1048576
)

// CacheHandler 缓存处理器
type CacheHandler struct {
	client            Client
	lruCacheTable     []int16
	staticRangeOldest map[string]int64
	cacheCount        int
	cacheSize         int64
	lruClearPointer   int
	lruSkipCheckCycle int
	pruneAggression   int
	lastFileVerification int64
	cacheLoaded       bool
}

// Client 客户端接口
type Client interface {
	GetServerHandler() *network.ServerHandler
	IsShuttingDown() bool
}

// NewCacheHandler 创建新的缓存处理器
func NewCacheHandler(client Client) (*CacheHandler, error) {
	settings := config.GetSettings()

	// 清理临时目录中的孤立文件
	tempDir := settings.GetTempDir()
	entries, err := os.ReadDir(tempDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				name := entry.Name()
				// 不删除日志和配置文件
				if !strings.HasPrefix(name, "log_") && !strings.HasPrefix(name, "pcache_") &&
					name != "client_login" && name != "hathcert.p12" {
					filePath := filepath.Join(tempDir, name)
					util.Debug("CacheHandler: 删除孤立的临时文件 %s", filePath)
					os.Remove(filePath)
				}
			}
		}
	}

	ch := &CacheHandler{
		client:            client,
		lruCacheTable:     make([]int16, LRU_CACHE_SIZE),
		staticRangeOldest: make(map[string]int64),
		cacheLoaded:       false,
	}

	// 初始化缓存
	if err := ch.initializeCache(); err != nil {
		return nil, err
	}

	return ch, nil
}

// initializeCache 初始化缓存
func (ch *CacheHandler) initializeCache() error {
	settings := config.GetSettings()
	cacheDir := settings.GetCacheDir()

	util.Info("CacheHandler: 初始化缓存系统...")

	// 扫描缓存目录计算总文件数和大小
	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 解析文件 ID
		fileID := info.Name()
		hvFile, err := hvfile.GetHVFileFromFileid(fileID)
		if err != nil {
			util.Debug("CacheHandler: 文件 %s 未被识别", path)
			os.Remove(path)
			return nil
		}

		// 检查是否在静态范围内
		if !settings.IsStaticRange(hvFile.GetStaticRange()) {
			util.Debug("CacheHandler: 文件 %s 不在活动静态范围内", path)
			os.Remove(path)
			return nil
		}

		// 检查文件大小
		if info.Size() != int64(hvFile.GetSize()) {
			util.Debug("CacheHandler: 文件 %s 已损坏", path)
			os.Remove(path)
			return nil
		}

		ch.addFileToActiveCache(hvFile)

		// 更新静态范围最旧时间
		staticRange := hvFile.GetStaticRange()
		if oldest, ok := ch.staticRangeOldest[staticRange]; !ok || info.ModTime().UnixNano() < oldest {
			ch.staticRangeOldest[staticRange] = info.ModTime().UnixNano()
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("扫描缓存失败: %w", err)
	}

	util.Info("CacheHandler: 完成缓存初始化 (%d 文件, %d 字节)", ch.cacheCount, ch.cacheSize)
	ch.updateStats()
	ch.cacheLoaded = true

	return nil
}

// addFileToActiveCache 添加文件到活动缓存
func (ch *CacheHandler) addFileToActiveCache(hvFile *hvfile.HVFile) {
	ch.cacheCount++
	ch.cacheSize += int64(hvFile.GetSize())
}

// updateStats 更新统计
func (ch *CacheHandler) updateStats() {
	settings := config.GetSettings()
	cacheSizeWithOverhead := ch.cacheSize + int64(ch.cacheCount)*settings.GetFileSystemBlockSize()/2

	stats.SetCacheCount(ch.cacheCount)
	stats.SetCacheSize(cacheSizeWithOverhead)
}

// GetCacheCount 获取缓存数量
func (ch *CacheHandler) GetCacheCount() int {
	return ch.cacheCount
}

// MarkRecentlyAccessed 标记最近访问
func (ch *CacheHandler) MarkRecentlyAccessed(hvFile *hvfile.HVFile) bool {
	return ch.markRecentlyAccessed(hvFile, false)
}

// markRecentlyAccessed 标记最近访问（内部）
func (ch *CacheHandler) markRecentlyAccessed(hvFile *hvfile.HVFile, skipMetaUpdate bool) bool {
	fileID := hvFile.GetFileID()

	// 位 16-35
	arrayIndex, _ := strconv.ParseInt(fileID[4:9], 16, 64)

	// 位 36-39 (十六进制字符 0-f)
	bitChar := fileID[9]
	var bitValue int
	switch {
	case bitChar >= '0' && bitChar <= '9':
		bitValue = int(bitChar - '0')
	case bitChar >= 'a' && bitChar <= 'f':
		bitValue = int(bitChar-'a') + 10
	case bitChar >= 'A' && bitChar <= 'F':
		bitValue = int(bitChar-'A') + 10
	default:
		bitValue = 0 // 无效字符，使用 0
	}
	bitMask := int16(1 << bitValue)

	markFile := true

	if (ch.lruCacheTable[arrayIndex] & bitMask) != 0 {
		markFile = false
	} else {
		ch.lruCacheTable[arrayIndex] |= bitMask
	}

	if markFile && !skipMetaUpdate {
		filePath := hvFile.GetLocalFileRef()
		info, err := os.Stat(filePath)
		if err == nil {
			oneWeekAgo := time.Now().Add(-7 * 24 * time.Hour)
			if info.ModTime().Before(oneWeekAgo) {
				os.Chtimes(filePath, time.Now(), time.Now())
			}
		}
	}

	return markFile
}

// IsFileVerificationOnCooldown 检查文件验证是否在冷却中
func (ch *CacheHandler) IsFileVerificationOnCooldown() bool {
	now := time.Now().UnixMilli()

	if ch.lastFileVerification > 0 && now-ch.lastFileVerification < 2000 {
		return true
	}

	ch.lastFileVerification = now
	return false
}

// DeleteFileFromCache 从缓存删除文件
func (ch *CacheHandler) DeleteFileFromCache(hvFile *hvfile.HVFile) {
	filePath := hvFile.GetLocalFileRef()

	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			util.Error("CacheHandler: 删除缓存文件失败: %v", err)
			return
		}

		ch.cacheCount--
		ch.cacheSize -= int64(hvFile.GetSize())
		ch.updateStats()

		util.Debug("CacheHandler: 删除缓存文件 %s", hvFile.String())
	}
}

// CycleLRUCacheTable 循环 LRU 缓存表
func (ch *CacheHandler) CycleLRUCacheTable() {
	clearUntil := minInt(LRU_CACHE_SIZE, ch.lruClearPointer+17)

	for ch.lruClearPointer < clearUntil {
		ch.lruCacheTable[ch.lruClearPointer] = 0
		ch.lruClearPointer++
	}

	if clearUntil >= LRU_CACHE_SIZE {
		ch.lruClearPointer = 0
	}
}

// RecheckFreeDiskSpace 重新检查磁盘空间
func (ch *CacheHandler) RecheckFreeDiskSpace() bool {
	if ch.lruSkipCheckCycle > 0 {
		ch.lruSkipCheckCycle--
		return true
	}

	settings := config.GetSettings()
	wantFree := int64(104857600) // 100MB
	cacheLimit := settings.GetDiskLimitBytes()
	cacheSizeWithOverhead := ch.getCacheSizeWithOverhead()
	bytesToFree := int64(0)

	if cacheSizeWithOverhead > cacheLimit {
		bytesToFree = wantFree + cacheSizeWithOverhead - cacheLimit
	} else if cacheLimit-cacheSizeWithOverhead < wantFree {
		bytesToFree = wantFree - (cacheLimit - cacheSizeWithOverhead)
	}

	if bytesToFree > 0 && ch.cacheCount > 0 && settings.GetStaticRangeCount() > 0 {
		// 找到最旧的静态范围并删除文件
		pruneStaticRange := ch.findOldestStaticRange()
		if pruneStaticRange == "" {
			util.Warning("CacheHandler: 找不到要修剪的静态范围")
			return false
		}

		// 删除该范围内的旧文件
		cacheDir := settings.GetCacheDir()
		staticRangeDir := filepath.Join(cacheDir, pruneStaticRange[0:2], pruneStaticRange[2:4])

		entries, err := os.ReadDir(staticRangeDir)
		if err != nil {
			util.Warning("CacheHandler: 无法访问静态范围目录 %s", staticRangeDir)
			// 更新时间戳以避免无限循环
			ch.staticRangeOldest[pruneStaticRange] = time.Now().UnixNano()
		} else {
			oldestTime := time.Now()

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				info, err := entry.Info()
				if err != nil {
					continue
				}

				fileID := entry.Name()
				hvFile, err := hvfile.GetHVFileFromFileid(fileID)
				if err == nil {
					ch.DeleteFileFromCache(hvFile)
					bytesToFree -= int64(hvFile.GetSize())
				}

				if info.ModTime().Before(oldestTime) {
					oldestTime = info.ModTime()
				}
			}

			ch.staticRangeOldest[pruneStaticRange] = oldestTime.UnixNano()
		}
	}

	ch.lruSkipCheckCycle = 60
	ch.pruneAggression = 1

	if settings.IsSkipFreeSpaceCheck() {
		return true
	}

	return true
}

// findOldestStaticRange 找到最旧的静态范围
func (ch *CacheHandler) findOldestStaticRange() string {
	var oldestRange string
	var oldestTime int64 = time.Now().UnixNano()

	for rangeName, rangeTime := range ch.staticRangeOldest {
		if rangeTime < oldestTime {
			oldestTime = rangeTime
			oldestRange = rangeName
		}
	}

	return oldestRange
}

// getCacheSizeWithOverhead 获取带开销的缓存大小
func (ch *CacheHandler) getCacheSizeWithOverhead() int64 {
	settings := config.GetSettings()
	return ch.cacheSize + int64(ch.cacheCount)*settings.GetFileSystemBlockSize()/2
}

// GetPruneAggression 获取修剪激进度
func (ch *CacheHandler) GetPruneAggression() int {
	return ch.pruneAggression
}

// ProcessBlacklist 处理黑名单
func (ch *CacheHandler) ProcessBlacklist(deltaTime int64) {
	util.Info("CacheHandler: 获取黑名单文件列表...")
	blacklisted := ch.client.GetServerHandler().GetBlacklist(deltaTime)

	if blacklisted == nil {
		util.Warning("CacheHandler: 获取文件黑名单失败，稍后重试")
		return
	}

	util.Info("CacheHandler: 查找并删除黑名单文件...")

	counter := 0
	for _, fileID := range blacklisted {
		hvFile, err := hvfile.GetHVFileFromFileid(fileID)
		if err != nil {
			continue
		}

		filePath := hvFile.GetLocalFileRef()
		if _, err := os.Stat(filePath); err == nil {
			ch.DeleteFileFromCache(hvFile)
			util.Debug("CacheHandler: 删除黑名单文件 %s", fileID)
			counter++
		}
	}

	util.Info("CacheHandler: 删除了 %d 个黑名单文件", counter)
}

// TerminateCache 终止缓存
func (ch *CacheHandler) TerminateCache() {
	_ = ch.savePersistentData()
	ch.cacheLoaded = false
}

// importFileToCache 导入文件到缓存
func (ch *CacheHandler) importFileToCache(tempFile string, hvFile *hvfile.HVFile) bool {
	targetPath := hvFile.GetLocalFileRef()

	// 创建目标目录
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		util.Warning("无法创建目标目录: %v", err)
		return false
	}

	// 移动文件
	if err := os.Rename(tempFile, targetPath); err != nil {
		// 如果重命名失败，尝试复制
		src, err := os.Open(tempFile)
		if err != nil {
			return false
		}
		defer src.Close()

		dst, err := os.Create(targetPath)
		if err != nil {
			return false
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return false
		}

		os.Remove(tempFile)
	}

	ch.addFileToActiveCache(hvFile)
	ch.markRecentlyAccessed(hvFile, true)

	// 检查静态范围最旧时间戳缓存
	staticRange := hvFile.GetStaticRange()
	if _, ok := ch.staticRangeOldest[staticRange]; !ok {
		util.Debug("CacheHandler: 为 %s 创建 staticRangeOldest 条目", staticRange)
		ch.staticRangeOldest[staticRange] = time.Now().UnixNano()
	}

	return true
}

// ImportFileToCache 导入文件到缓存（导出接口）
func (ch *CacheHandler) ImportFileToCache(tempFile string, hvFile *hvfile.HVFile) bool {
	return ch.importFileToCache(tempFile, hvFile)
}

// getFileSHA1 获取文件 SHA-1
func getFileSHA1(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha1.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return strings.ToLower(hex.EncodeToString(hasher.Sum(nil))), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
