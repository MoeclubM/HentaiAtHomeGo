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
	persistentInfoFile = "pcache_go_info"
	persistentLRUFile  = "pcache_go_lru"
	persistentAgesFile = "pcache_go_ages"
)

func (ch *CacheHandler) savePersistentData() error {
	if !ch.cacheLoaded {
		return nil
	}

	settings := config.GetSettings()
	agesPath := filepath.Join(settings.GetDataDir(), persistentAgesFile)
	agesHash, err := writeCacheObject(agesPath, ch.staticRangeOldest)
	if err != nil {
		return fmt.Errorf("保存 ages 失败: %w", err)
	}

	lruPath := filepath.Join(settings.GetDataDir(), persistentLRUFile)
	lruHash, err := writeCacheObject(lruPath, ch.lruCacheTable)
	if err != nil {
		return fmt.Errorf("保存 lru 失败: %w", err)
	}

	infoContent := fmt.Sprintf("cacheCount=%d\ncacheSize=%d\nlruClearPointer=%d\nagesHash=%s\nlruHash=%s",
		ch.cacheCount, ch.cacheSize, ch.lruClearPointer, agesHash, lruHash)
	infoPath := filepath.Join(settings.GetDataDir(), persistentInfoFile)
	if err := util.PutStringFileContents(infoPath, infoContent); err != nil {
		return fmt.Errorf("保存 info 失败: %w", err)
	}

	return nil
}

func (ch *CacheHandler) loadPersistentData() (bool, error) {
	settings := config.GetSettings()
	infoPath := filepath.Join(settings.GetDataDir(), persistentInfoFile)
	if _, err := os.Stat(infoPath); os.IsNotExist(err) {
		util.Debug("CacheHandler: 缺少持久化 info 文件，强制重新扫描")
		return false, nil
	}

	content, err := util.GetStringFileContents(infoPath)
	if err != nil {
		return false, fmt.Errorf("读取持久化 info 失败: %w", err)
	}

	infoChecksum := 0
	var agesHash, lruHash string
	for _, line := range strings.Split(content, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "cacheCount":
			if _, err := fmt.Sscanf(parts[1], "%d", &ch.cacheCount); err == nil {
				infoChecksum |= 1
			}
		case "cacheSize":
			if _, err := fmt.Sscanf(parts[1], "%d", &ch.cacheSize); err == nil {
				infoChecksum |= 2
			}
		case "lruClearPointer":
			if _, err := fmt.Sscanf(parts[1], "%d", &ch.lruClearPointer); err == nil {
				infoChecksum |= 4
			}
		case "agesHash":
			agesHash = parts[1]
			infoChecksum |= 8
		case "lruHash":
			lruHash = parts[1]
			infoChecksum |= 16
		}
	}

	_ = os.Remove(infoPath)
	if infoChecksum != 31 {
		return false, nil
	}

	agesPath := filepath.Join(settings.GetDataDir(), persistentAgesFile)
	ages, err := readCacheObject[map[string]int64](agesPath, agesHash)
	if err != nil {
		return false, err
	}
	if len(ages) > settings.GetStaticRangeCount() {
		return false, nil
	}

	lruPath := filepath.Join(settings.GetDataDir(), persistentLRUFile)
	lru, err := readCacheObject[[]int16](lruPath, lruHash)
	if err != nil {
		return false, err
	}
	if len(lru) != LRU_CACHE_SIZE {
		return false, nil
	}

	ch.staticRangeOldest = ages
	ch.lruCacheTable = lru
	ch.updateStats()
	return true, nil
}

func writeCacheObject[T any](filePath string, value T) (string, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}

	encoder := gob.NewEncoder(file)
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil {
		return "", encodeErr
	}
	if closeErr != nil {
		return "", closeErr
	}

	return util.GetSHA1File(filePath)
}

func readCacheObject[T any](filePath, expectedHash string) (T, error) {
	var zero T
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return zero, fmt.Errorf("持久化文件不存在: %s", filePath)
	}

	hash, err := util.GetSHA1File(filePath)
	if err != nil {
		return zero, err
	}
	if hash != expectedHash {
		return zero, fmt.Errorf("持久化文件哈希不匹配: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return zero, err
	}
	defer file.Close()

	var value T
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&value); err != nil {
		return zero, err
	}

	return value, nil
}

func (ch *CacheHandler) deletePersistentData() {
	settings := config.GetSettings()
	_ = os.Remove(filepath.Join(settings.GetDataDir(), persistentInfoFile))
	_ = os.Remove(filepath.Join(settings.GetDataDir(), persistentAgesFile))
	_ = os.Remove(filepath.Join(settings.GetDataDir(), persistentLRUFile))
}
