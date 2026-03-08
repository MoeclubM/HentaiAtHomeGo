package cache

import (
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

const LRU_CACHE_SIZE = 1048576

type CacheHandler struct {
	client               Client
	lruCacheTable        []int16
	staticRangeOldest    map[string]int64
	cacheCount           int
	cacheSize            int64
	lruClearPointer      int
	lruSkipCheckCycle    int
	pruneAggression      int
	lastFileVerification int64
	cacheLoaded          bool
}

type Client interface {
	GetServerHandler() *network.ServerHandler
	IsShuttingDown() bool
}

func NewCacheHandler(client Client) (*CacheHandler, error) {
	settings := config.GetSettings()

	entries, err := os.ReadDir(settings.GetTempDir())
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			if strings.HasPrefix(name, "log_") || strings.HasPrefix(name, "pcache_") || name == "client_login" || name == "hathcert.p12" {
				continue
			}

			filePath := filepath.Join(settings.GetTempDir(), name)
			util.Debug("CacheHandler: 删除孤立临时文件 %s", filePath)
			_ = os.Remove(filePath)
		}
	}

	ch := &CacheHandler{
		client:            client,
		lruCacheTable:     make([]int16, LRU_CACHE_SIZE),
		staticRangeOldest: make(map[string]int64),
		pruneAggression:   1,
	}

	if err := ch.initializeCache(); err != nil {
		return nil, err
	}

	return ch, nil
}

func (ch *CacheHandler) initializeCache() error {
	settings := config.GetSettings()
	fastStartup := false

	if !settings.IsRescanCache() {
		util.Info("CacheHandler: 尝试加载持久化缓存数据...")
		loaded, err := ch.loadPersistentData()
		if err != nil {
			util.Debug("CacheHandler: 持久化缓存加载失败，将回退到全量扫描: %v", err)
		} else if loaded {
			util.Info("CacheHandler: 已加载持久化缓存数据")
			fastStartup = true
		} else {
			util.Info("CacheHandler: 持久化缓存数据不可用")
		}
	}

	ch.deletePersistentData()

	if !fastStartup {
		util.Info("CacheHandler: 初始化缓存系统...")
		if err := ch.startupCacheCleanup(); err != nil {
			return err
		}

		if ch.client.IsShuttingDown() {
			return nil
		}

		ch.lruClearPointer = 0
		ch.cacheCount = 0
		ch.cacheSize = 0
		ch.staticRangeOldest = make(map[string]int64, max(1, int(float64(settings.GetStaticRangeCount())*1.5)))
		ch.lruCacheTable = make([]int16, LRU_CACHE_SIZE)

		if err := ch.startupInitCache(); err != nil {
			return err
		}
	}

	ch.cacheLoaded = true

	if !ch.RecheckFreeDiskSpace() {
		return fmt.Errorf("缓存所在存储设备剩余空间不足，无法满足当前节点运行要求")
	}

	if ch.cacheCount < 1 && settings.GetStaticRangeCount() > 20 {
		return fmt.Errorf("当前节点已分配静态范围，但缓存为空；这会严重影响 CDN 信任度，请检查缓存目录或在管理端重置静态范围")
	}

	return nil
}

func (ch *CacheHandler) startupCacheCleanup() error {
	settings := config.GetSettings()
	cacheDir := settings.GetCacheDir()

	util.Info("CacheHandler: 启动缓存清理扫描...")
	level1Entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return fmt.Errorf("无法访问缓存目录 %s: %w", cacheDir, err)
	}

	if settings.GetStaticRangeCount() > 0 && len(level1Entries) > settings.GetStaticRangeCount() {
		util.Warning("CacheHandler: 缓存目录下存在 %d 个一级目录，但当前只分配了 %d 个静态范围", len(level1Entries), settings.GetStaticRangeCount())
	}

	checkedCounter := 0
	checkedCounterPct := 0

	for _, l1entry := range level1Entries {
		if ch.client.IsShuttingDown() {
			return nil
		}

		l1path := filepath.Join(cacheDir, l1entry.Name())
		if !l1entry.IsDir() {
			_ = os.Remove(l1path)
			continue
		}

		level2Entries, err := os.ReadDir(l1path)
		if err != nil {
			util.Warning("CacheHandler: 无法访问一级缓存目录 %s", l1path)
			continue
		}

		if len(level2Entries) == 0 {
			_ = os.Remove(l1path)
			continue
		}

		for _, l2entry := range level2Entries {
			if l2entry.IsDir() {
				continue
			}

			l2path := filepath.Join(l1path, l2entry.Name())
			hvFile, err := hvfile.GetHVFileFromFile(l2path)
			if err != nil {
				util.Debug("CacheHandler: 文件 %s 无法识别，已删除", l2path)
				_ = os.Remove(l2path)
				continue
			}

			if !settings.IsStaticRange(hvFile.GetFileID()) {
				util.Debug("CacheHandler: 文件 %s 不在当前活动静态范围内，已删除", l2path)
				_ = os.Remove(l2path)
				continue
			}

			if ch.moveFileToCacheDir(l2path, hvFile) {
				util.Debug("CacheHandler: 已重定位文件 %s 到 %s", hvFile.GetFileID(), hvFile.GetLocalFileRef())
			}
		}

		checkedCounter++
		if len(level1Entries) > 9 {
			pct := checkedCounter * 100 / len(level1Entries)
			if pct >= checkedCounterPct+10 {
				checkedCounterPct += 10
				util.Info("CacheHandler: 启动清理进度 %d%%", checkedCounterPct)
			}
		}
	}

	util.Info("CacheHandler: 已扫描 %d 个一级缓存目录", checkedCounter)
	return nil
}

func (ch *CacheHandler) startupInitCache() error {
	settings := config.GetSettings()
	cacheDir := settings.GetCacheDir()

	var validator hvfile.FileValidator
	printFreq := 10000
	if settings.IsVerifyCache() {
		util.Info("CacheHandler: 启动时执行完整缓存校验，这可能持续较长时间")
		validator = util.NewFileValidator()
		printFreq = 1000
	} else {
		util.Info("CacheHandler: 加载缓存...")
	}

	recentlyAccessedCutoff := time.Now().Add(-7 * 24 * time.Hour).UnixNano()
	foundStaticRanges := 0

	level1Entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return fmt.Errorf("无法访问缓存目录 %s: %w", cacheDir, err)
	}

	for _, l1entry := range level1Entries {
		if ch.client.IsShuttingDown() {
			return nil
		}

		if !l1entry.IsDir() {
			continue
		}

		l1path := filepath.Join(cacheDir, l1entry.Name())
		level2Entries, err := os.ReadDir(l1path)
		if err != nil {
			continue
		}

		for _, l2entry := range level2Entries {
			if !l2entry.IsDir() {
				continue
			}

			l2path := filepath.Join(l1path, l2entry.Name())
			files, err := os.ReadDir(l2path)
			if err != nil {
				util.Warning("CacheHandler: 无法访问二级缓存目录 %s", l2path)
				continue
			}

			filesInDir := 0
			oldestLastModified := time.Now().UnixNano()

			for _, entry := range files {
				if !entry.Type().IsRegular() {
					continue
				}

				filesInDir++
				filePath := filepath.Join(l2path, entry.Name())
				info, err := entry.Info()
				if err != nil {
					_ = os.Remove(filePath)
					filesInDir--
					continue
				}

				var hvFile *hvfile.HVFile
				if validator != nil {
					hvFile, err = hvfile.GetHVFileFromFileWithValidator(filePath, validator)
				} else {
					hvFile, err = hvfile.GetHVFileFromFile(filePath)
				}

				if err != nil {
					util.Debug("CacheHandler: 文件 %s 已损坏，已删除", filePath)
					_ = os.Remove(filePath)
					filesInDir--
					continue
				}

				if !settings.IsStaticRange(hvFile.GetFileID()) {
					util.Debug("CacheHandler: 文件 %s 不在活动静态范围内，已删除", filePath)
					_ = os.Remove(filePath)
					filesInDir--
					continue
				}

				ch.addFileToActiveCache(hvFile)
				fileLastModified := info.ModTime().UnixNano()
				if fileLastModified > recentlyAccessedCutoff {
					ch.markRecentlyAccessed(hvFile, true)
				}
				if fileLastModified < oldestLastModified {
					oldestLastModified = fileLastModified
				}

				if ch.cacheCount%printFreq == 0 {
					util.Info("CacheHandler: 已加载 %d 个缓存文件...", ch.cacheCount)
				}
			}

			if filesInDir < 1 {
				_ = os.Remove(l2path)
				continue
			}

			staticRange := l1entry.Name() + l2entry.Name()
			ch.staticRangeOldest[staticRange] = oldestLastModified
			foundStaticRanges++
			if foundStaticRanges%100 == 0 {
				util.Info("CacheHandler: 已发现 %d 个含缓存文件的静态范围...", foundStaticRanges)
			}
		}
	}

	util.Info("CacheHandler: 缓存初始化完成（%d 文件，%d 表观字节，%d 估算磁盘字节）", ch.cacheCount, ch.cacheSize, ch.getCacheSizeWithOverhead())
	util.Info("CacheHandler: 共发现 %d 个含缓存文件的静态范围", foundStaticRanges)
	ch.updateStats()
	return nil
}

func (ch *CacheHandler) addFileToActiveCache(hvFile *hvfile.HVFile) {
	ch.cacheCount++
	ch.cacheSize += int64(hvFile.GetSize())
}

func (ch *CacheHandler) updateStats() {
	stats.SetCacheCount(ch.cacheCount)
	stats.SetCacheSize(ch.getCacheSizeWithOverhead())
}

func (ch *CacheHandler) GetCacheCount() int {
	return ch.cacheCount
}

func (ch *CacheHandler) MarkRecentlyAccessed(hvFile *hvfile.HVFile) bool {
	return ch.markRecentlyAccessed(hvFile, false)
}

func (ch *CacheHandler) markRecentlyAccessed(hvFile *hvfile.HVFile, skipMetaUpdate bool) bool {
	fileID := hvFile.GetFileID()
	arrayIndex, _ := strconv.ParseInt(fileID[4:9], 16, 64)

	bitChar := fileID[9]
	bitValue := 0
	switch {
	case bitChar >= '0' && bitChar <= '9':
		bitValue = int(bitChar - '0')
	case bitChar >= 'a' && bitChar <= 'f':
		bitValue = int(bitChar-'a') + 10
	case bitChar >= 'A' && bitChar <= 'F':
		bitValue = int(bitChar-'A') + 10
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
				now := time.Now()
				_ = os.Chtimes(filePath, now, now)
			}
		}
	}

	return markFile
}

func (ch *CacheHandler) IsFileVerificationOnCooldown() bool {
	now := time.Now().UnixMilli()
	if ch.lastFileVerification > 0 && now-ch.lastFileVerification < 2000 {
		return true
	}
	ch.lastFileVerification = now
	return false
}

func (ch *CacheHandler) DeleteFileFromCache(hvFile *hvfile.HVFile) {
	filePath := hvFile.GetLocalFileRef()
	if _, err := os.Stat(filePath); err != nil {
		return
	}

	if err := os.Remove(filePath); err != nil {
		util.Error("CacheHandler: 删除缓存文件失败: %v", err)
		return
	}

	ch.cacheCount--
	ch.cacheSize -= int64(hvFile.GetSize())
	ch.updateStats()
	util.Debug("CacheHandler: 删除缓存文件 %s", hvFile.String())
}

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

func (ch *CacheHandler) RecheckFreeDiskSpace() bool {
	if ch.lruSkipCheckCycle > 0 {
		ch.lruSkipCheckCycle--
		return true
	}

	settings := config.GetSettings()
	wantFree := int64(104857600)
	cacheLimit := settings.GetDiskLimitBytes()
	cacheSizeWithOverhead := ch.getCacheSizeWithOverhead()
	bytesToFree := int64(0)

	if cacheSizeWithOverhead > cacheLimit {
		bytesToFree = wantFree + cacheSizeWithOverhead - cacheLimit
	} else if cacheLimit-cacheSizeWithOverhead < wantFree {
		bytesToFree = wantFree - (cacheLimit - cacheSizeWithOverhead)
	}

	util.Debug("CacheHandler: 检查缓存空间 (cacheSize=%d cacheSizeWithOverhead=%d cacheLimit=%d cacheFree=%d)", ch.cacheSize, cacheSizeWithOverhead, cacheLimit, cacheLimit-cacheSizeWithOverhead)

	if bytesToFree > 0 && ch.cacheCount > 0 && settings.GetStaticRangeCount() > 0 {
		pruneStaticRange := ch.findOldestStaticRange()
		if pruneStaticRange == "" {
			util.Warning("CacheHandler: 无法找到要修剪的静态范围")
			return false
		}

		now := time.Now()
		oldestRangeAge := time.Unix(0, ch.staticRangeOldest[pruneStaticRange])
		lruLastModifiedPruneCutoff := oldestRangeAge

		switch {
		case oldestRangeAge.Before(now.Add(-180 * 24 * time.Hour)):
			lruLastModifiedPruneCutoff = lruLastModifiedPruneCutoff.Add(30 * 24 * time.Hour)
		case oldestRangeAge.Before(now.Add(-90 * 24 * time.Hour)):
			lruLastModifiedPruneCutoff = lruLastModifiedPruneCutoff.Add(7 * 24 * time.Hour)
		case oldestRangeAge.Before(now.Add(-30 * 24 * time.Hour)):
			lruLastModifiedPruneCutoff = lruLastModifiedPruneCutoff.Add(3 * 24 * time.Hour)
		default:
			lruLastModifiedPruneCutoff = lruLastModifiedPruneCutoff.Add(24 * time.Hour)
		}

		util.Debug("CacheHandler: 尝试释放 %d 字节，当前扫描静态范围 %s", bytesToFree, pruneStaticRange)

		staticRangeDir := filepath.Join(settings.GetCacheDir(), pruneStaticRange[0:2], pruneStaticRange[2:4])
		entries, err := os.ReadDir(staticRangeDir)
		if err != nil {
			util.Warning("CacheHandler: 无法访问静态范围目录 %s", staticRangeDir)
			ch.staticRangeOldest[pruneStaticRange] = lruLastModifiedPruneCutoff.UnixNano()
		} else {
			oldestLastModified := now.UnixNano()
			util.Debug("CacheHandler: 检查 %d 个文件，修剪阈值=%d", len(entries), lruLastModifiedPruneCutoff.UnixNano())

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				filePath := filepath.Join(staticRangeDir, entry.Name())
				info, err := entry.Info()
				if err != nil {
					continue
				}

				lastModified := info.ModTime().UnixNano()
				if lastModified < lruLastModifiedPruneCutoff.UnixNano() {
					hvFile, err := hvfile.GetHVFileFromFileid(entry.Name())
					if err != nil {
						util.Warning("CacheHandler: 删除无效缓存文件 %s", filePath)
						_ = os.Remove(filePath)
						continue
					}

					ch.DeleteFileFromCache(hvFile)
					bytesToFree -= int64(hvFile.GetSize())
					util.Debug("CacheHandler: 已修剪文件 lastModified=%d size=%d bytesToFree=%d cacheCount=%d", lastModified, hvFile.GetSize(), bytesToFree, ch.cacheCount)
				} else if lastModified < oldestLastModified {
					oldestLastModified = lastModified
				}
			}

			ch.staticRangeOldest[pruneStaticRange] = oldestLastModified
			util.Debug("CacheHandler: 静态范围 %s 的最旧时间已更新为 %d", pruneStaticRange, oldestLastModified)
		}
	} else {
		if cacheLimit-cacheSizeWithOverhead > wantFree*10 {
			ch.lruSkipCheckCycle = 60
		} else {
			ch.lruSkipCheckCycle = 6
		}
	}

	if bytesToFree > 10485760 {
		ch.pruneAggression = int(bytesToFree / 10485760)
	} else {
		ch.pruneAggression = 1
	}

	if settings.IsSkipFreeSpaceCheck() {
		util.Debug("CacheHandler: 已禁用磁盘剩余空间检查")
		return true
	}

	diskFreeSpace, err := getDiskFreeSpace(settings.GetCacheDir())
	if err != nil {
		util.Warning("CacheHandler: 获取磁盘剩余空间失败: %v", err)
		return false
	}

	minRequired := maxInt64(settings.GetDiskMinRemainingBytes(), wantFree)
	if int64(diskFreeSpace) < minRequired {
		util.Warning("CacheHandler: 未满足剩余空间约束，设备可用空间仅 %d 字节", diskFreeSpace)
		return false
	}

	util.Debug("CacheHandler: 已满足磁盘剩余空间约束，可用空间 %d 字节", diskFreeSpace)
	return true
}

func (ch *CacheHandler) findOldestStaticRange() string {
	var oldestRange string
	oldestTime := time.Now().UnixNano()
	for rangeName, rangeTime := range ch.staticRangeOldest {
		if rangeTime < oldestTime {
			oldestTime = rangeTime
			oldestRange = rangeName
		}
	}
	return oldestRange
}

func (ch *CacheHandler) getCacheSizeWithOverhead() int64 {
	return ch.cacheSize + int64(ch.cacheCount)*config.GetSettings().GetFileSystemBlockSize()/2
}

func (ch *CacheHandler) GetPruneAggression() int {
	return ch.pruneAggression
}

func (ch *CacheHandler) ProcessBlacklist(deltaTime int64) {
	util.Info("CacheHandler: 获取黑名单文件列表...")
	blacklisted := ch.client.GetServerHandler().GetBlacklist(deltaTime)
	if blacklisted == nil {
		util.Warning("CacheHandler: 获取黑名单失败，稍后重试")
		return
	}

	util.Info("CacheHandler: 查找并删除黑名单文件...")
	counter := 0
	for _, fileID := range blacklisted {
		hvFile, err := hvfile.GetHVFileFromFileid(fileID)
		if err != nil {
			continue
		}

		if _, err := os.Stat(hvFile.GetLocalFileRef()); err == nil {
			ch.DeleteFileFromCache(hvFile)
			util.Debug("CacheHandler: 删除黑名单文件 %s", fileID)
			counter++
		}
	}

	util.Info("CacheHandler: 删除了 %d 个黑名单文件", counter)
}

func (ch *CacheHandler) TerminateCache() {
	_ = ch.savePersistentData()
	ch.cacheLoaded = false
}

func (ch *CacheHandler) importFileToCache(tempFile string, hvFile *hvfile.HVFile) bool {
	if !ch.moveFileToCacheDir(tempFile, hvFile) {
		return false
	}

	ch.addFileToActiveCache(hvFile)
	ch.markRecentlyAccessed(hvFile, true)

	staticRange := hvFile.GetStaticRange()
	if _, ok := ch.staticRangeOldest[staticRange]; !ok {
		util.Debug("CacheHandler: 为 %s 创建 staticRangeOldest 条目", staticRange)
		ch.staticRangeOldest[staticRange] = time.Now().UnixNano()
	}

	return true
}

func (ch *CacheHandler) ImportFileToCache(tempFile string, hvFile *hvfile.HVFile) bool {
	return ch.importFileToCache(tempFile, hvFile)
}

func (ch *CacheHandler) moveFileToCacheDir(sourcePath string, hvFile *hvfile.HVFile) bool {
	targetPath := hvFile.GetLocalFileRef()
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		util.Warning("CacheHandler: 无法创建目标目录: %v", err)
		return false
	}

	_ = os.Remove(targetPath)
	if err := os.Rename(sourcePath, targetPath); err == nil {
		if _, err := os.Stat(targetPath); err == nil {
			return true
		}
	}

	src, err := os.Open(sourcePath)
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

	_ = os.Remove(sourcePath)
	_, err = os.Stat(targetPath)
	return err == nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
