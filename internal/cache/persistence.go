// Package cache 提供缓存持久化功能
package cache

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/util"
)

const (
	// 仅用于 Go 版自身的持久化加速；不与 Java 的 pcache_* 互相兼容。
	//（本次按你的要求：只保证网络协议兼容，不强求本地文件兼容。）
	persistentInfoFile = "pcache_go_info"
	persistentLRUFile  = "pcache_go_lru"
	persistentAgesFile = "pcache_go_ages"
)

// savePersistentData 保存持久化数据
func (ch *CacheHandler) savePersistentData() error {
	if !ch.cacheLoaded {
		return nil
	}

	settings := config.GetSettings()

	// 保存 staticRangeOldest
	agesFile := filepath.Join(settings.GetDataDir(), persistentAgesFile)
	agesHash, err := ch.writeCacheObject(agesFile, ch.staticRangeOldest)
	if err != nil {
		return fmt.Errorf("保存 ages 失败: %w", err)
	}

	// 保存 lruCacheTable
	lruFile := filepath.Join(settings.GetDataDir(), persistentLRUFile)
	lruHash, err := ch.writeCacheObject(lruFile, ch.lruCacheTable)
	if err != nil {
		return fmt.Errorf("保存 LRU 失败: %w", err)
	}

	// 保存 info 文件
	infoContent := fmt.Sprintf("cacheCount=%d\ncacheSize=%d\nlruClearPointer=%d\nagesHash=%s\nlruHash=%s",
		ch.cacheCount, ch.cacheSize, ch.lruClearPointer, agesHash, lruHash)
	infoFile := filepath.Join(settings.GetDataDir(), persistentInfoFile)
	if err := util.PutStringFileContents(infoFile, infoContent); err != nil {
		return fmt.Errorf("保存 info 失败: %w", err)
	}

	util.Debug("保存持久化数据完成")
	return nil
}

// loadPersistentData 加载持久化数据
func (ch *CacheHandler) loadPersistentData() (bool, error) {
	settings := config.GetSettings()
	infoFile := filepath.Join(settings.GetDataDir(), persistentInfoFile)

	// 检查 info 文件是否存在
	if _, err := os.Stat(infoFile); os.IsNotExist(err) {
		util.Debug("CacheHandler: 缺少 pcache_info，强制重新扫描")
		return false, nil
	}

	// 读取 info 文件
	content, err := util.GetStringFileContents(infoFile)
	if err != nil {
		return false, fmt.Errorf("读取 info 失败: %w", err)
	}

	// 解析 info 文件
	infoChecksum := 0
	var agesHash, lruHash string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "cacheCount":
			if _, err := fmt.Sscanf(parts[1], "%d", &ch.cacheCount); err == nil {
				util.Debug("CacheHandler: 加载持久化 cacheCount=%d", ch.cacheCount)
				infoChecksum |= 1
			}
		case "cacheSize":
			if _, err := fmt.Sscanf(parts[1], "%d", &ch.cacheSize); err == nil {
				util.Debug("CacheHandler: 加载持久化 cacheSize=%d", ch.cacheSize)
				infoChecksum |= 2
			}
		case "lruClearPointer":
			if _, err := fmt.Sscanf(parts[1], "%d", &ch.lruClearPointer); err == nil {
				util.Debug("CacheHandler: 加载持久化 lruClearPointer=%d", ch.lruClearPointer)
				infoChecksum |= 4
			}
		case "agesHash":
			agesHash = parts[1]
			util.Debug("CacheHandler: 发现 agesHash=%s", agesHash)
			infoChecksum |= 8
		case "lruHash":
			lruHash = parts[1]
			util.Debug("CacheHandler: 发现 lruHash=%s", lruHash)
			infoChecksum |= 16
		}
	}

	// 删除 info 文件（防止反序列化失败）
	os.Remove(infoFile)

	if infoChecksum != 31 {
		util.Info("CacheHandler: 持久化字段缺失，强制重新扫描")
		return false, nil
	}

	// 加载 staticRangeOldest
	agesFile := filepath.Join(settings.GetDataDir(), persistentAgesFile)
	staticRangeOldest, err := ch.readCacheObject(agesFile, agesHash)
	if err != nil {
		return false, err
	}

	if sr, ok := staticRangeOldest.(map[string]int64); ok {
		ch.staticRangeOldest = sr
	} else {
		return false, fmt.Errorf("staticRangeOldest 类型错误")
	}

	// 检查静态范围数量
	if len(ch.staticRangeOldest) > settings.GetStaticRangeCount() {
		util.Info("CacheHandler: 缓存的静态范围数量大于当前分配的数量，强制重新扫描")
		return false, nil
	}

	util.Info("CacheHandler: 加载静态范围时间")

	// 加载 lruCacheTable
	lruFile := filepath.Join(settings.GetDataDir(), persistentLRUFile)
	lruCacheTable, err := ch.readCacheObject(lruFile, lruHash)
	if err != nil {
		return false, err
	}

	if lr, ok := lruCacheTable.([]int16); ok {
		ch.lruCacheTable = lr
	} else {
		return false, fmt.Errorf("lruCacheTable 类型错误")
	}

	util.Info("CacheHandler: 加载 LRU 缓存")
	ch.updateStats()

	return true, nil
}

// writeCacheObject 写入缓存对象
func (ch *CacheHandler) writeCacheObject(filePath string, object interface{}) (string, error) {
	util.Debug("写入缓存对象 %s", filePath)

	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(object); err != nil {
		return "", err
	}

	// 计算哈希
	hash, err := util.GetSHA1File(filePath)
	if err != nil {
		return "", err
	}

	util.Debug("写入缓存对象 %s，大小=%d 哈希=%s", filePath, getFileSize(filePath), hash)
	return hash, nil
}

// readCacheObject 读取缓存对象
func (ch *CacheHandler) readCacheObject(filePath, expectedHash string) (interface{}, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		util.Warning("CacheHandler: 缺少 %s，强制重新扫描", filePath)
		return nil, fmt.Errorf("文件不存在")
	}

	// 验证哈希
	hash, err := util.GetSHA1File(filePath)
	if err != nil {
		return "", err
	}

	if hash != expectedHash {
		util.Warning("CacheHandler: 读取 %s 时文件哈希不正确，强制重新扫描", filePath)
		return "", fmt.Errorf("哈希不匹配")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	var object interface{}
	if err := decoder.Decode(&object); err != nil {
		return "", err
	}

	return object, nil
}

// deletePersistentData 删除持久化数据
func (ch *CacheHandler) deletePersistentData() {
	settings := config.GetSettings()

	infoFile := filepath.Join(settings.GetDataDir(), persistentInfoFile)
	agesFile := filepath.Join(settings.GetDataDir(), persistentAgesFile)
	lruFile := filepath.Join(settings.GetDataDir(), persistentLRUFile)

	os.Remove(infoFile)
	os.Remove(agesFile)
	os.Remove(lruFile)
}

// getFileSize 获取文件大小
func getFileSize(filePath string) int64 {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0
	}
	return info.Size()
}
