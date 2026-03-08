package cache

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/network"
	"github.com/qwq/hentaiathomego/pkg/hvfile"
)

type cacheTestClient struct{}

func (cacheTestClient) GetServerHandler() *network.ServerHandler { return nil }
func (cacheTestClient) IsShuttingDown() bool                     { return false }

func configureCacheTestWithLimit(t *testing.T, staticRanges string, diskLimit string) string {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	logDir := filepath.Join(root, "log")
	cacheDir := filepath.Join(root, "cache")
	tempDir := filepath.Join(root, "tmp")
	downloadDir := filepath.Join(root, "download")

	for _, dir := range []string{dataDir, logDir, cacheDir, tempDir, downloadDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	settings := config.GetSettings()
	settings.ParseAndUpdateSettings([]string{
		"data_dir=" + dataDir,
		"log_dir=" + logDir,
		"cache_dir=" + cacheDir,
		"temp_dir=" + tempDir,
		"download_dir=" + downloadDir,
		"static_ranges=" + staticRanges,
		"disklimit_bytes=" + diskLimit,
		"diskremaining_bytes=0",
		"filesystem_blocksize=0",
		"skip_free_space_check=true",
		"verify_cache=false",
		"rescan_cache=true",
	})

	return root
}

func configureCacheTest(t *testing.T, staticRanges string) string {
	t.Helper()
	return configureCacheTestWithLimit(t, staticRanges, "1073741824")
}

func writeFileSized(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func forceDiskLimitBytes(limit int64) {
	settings := config.GetSettings()
	value := reflect.ValueOf(settings).Elem().FieldByName("diskLimitBytes")
	reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem().SetInt(limit)
}

func TestStartupCacheCleanupRelocatesFirstLevelFile(t *testing.T) {
	configureCacheTest(t, "abcd")
	fileID := "abcd000000000000000000000000000000000000-4-jpg"
	legacyPath := filepath.Join(config.GetSettings().GetCacheDir(), "ab", fileID)
	writeFileSized(t, legacyPath, []byte("test"))

	ch, err := NewCacheHandler(cacheTestClient{})
	if err != nil {
		t.Fatalf("NewCacheHandler: %v", err)
	}

	hvFile, err := hvfile.GetHVFileFromFileid(fileID)
	if err != nil {
		t.Fatalf("GetHVFileFromFileid: %v", err)
	}

	if _, err := os.Stat(hvFile.GetLocalFileRef()); err != nil {
		t.Fatalf("expected relocated file to exist: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy path to be gone, got err=%v", err)
	}
	if ch.GetCacheCount() != 1 {
		t.Fatalf("expected cache count 1, got %d", ch.GetCacheCount())
	}
}

func TestVerifyCacheDeletesCorruptFilesOnStartup(t *testing.T) {
	configureCacheTest(t, "aaaa")
	settings := config.GetSettings()
	settings.ParseAndUpdateSettings([]string{"verify_cache=true"})

	fileID := strings.Repeat("a", 40) + "-4-jpg"
	hvFile, err := hvfile.GetHVFileFromFileid(fileID)
	if err != nil {
		t.Fatalf("GetHVFileFromFileid: %v", err)
	}
	writeFileSized(t, hvFile.GetLocalFileRef(), []byte("bbbb"))

	ch, err := NewCacheHandler(cacheTestClient{})
	if err != nil {
		t.Fatalf("NewCacheHandler: %v", err)
	}

	if ch.GetCacheCount() != 0 {
		t.Fatalf("expected corrupt file to be removed, cache count=%d", ch.GetCacheCount())
	}
	if _, err := os.Stat(hvFile.GetLocalFileRef()); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt file to be deleted, got err=%v", err)
	}
}

func TestLoadPersistentDataEnablesFastStartup(t *testing.T) {
	configureCacheTest(t, "abcd")
	settings := config.GetSettings()
	settings.ParseAndUpdateSettings([]string{"rescan_cache=false"})

	fileID := "abcd000000000000000000000000000000000000-4-jpg"
	hvFile, err := hvfile.GetHVFileFromFileid(fileID)
	if err != nil {
		t.Fatalf("GetHVFileFromFileid: %v", err)
	}
	writeFileSized(t, hvFile.GetLocalFileRef(), []byte("test"))

	seed := &CacheHandler{
		client:            cacheTestClient{},
		lruCacheTable:     make([]int16, LRU_CACHE_SIZE),
		staticRangeOldest: map[string]int64{"abcd": time.Now().Add(-time.Hour).UnixNano()},
		cacheCount:        1,
		cacheSize:         int64(hvFile.GetSize()),
		lruClearPointer:   123,
		cacheLoaded:       true,
	}
	seed.lruCacheTable[10] = 7
	if err := seed.savePersistentData(); err != nil {
		t.Fatalf("savePersistentData: %v", err)
	}

	loaded, err := NewCacheHandler(cacheTestClient{})
	if err != nil {
		t.Fatalf("NewCacheHandler: %v", err)
	}

	if loaded.cacheCount != 1 {
		t.Fatalf("expected cache count 1, got %d", loaded.cacheCount)
	}
	if loaded.lruClearPointer != 123 {
		t.Fatalf("expected lruClearPointer 123, got %d", loaded.lruClearPointer)
	}
	if loaded.lruCacheTable[10] != 7 {
		t.Fatalf("expected persisted lru bit to load, got %d", loaded.lruCacheTable[10])
	}
	if len(loaded.staticRangeOldest) != 1 {
		t.Fatalf("expected one static range age, got %d", len(loaded.staticRangeOldest))
	}
}

func TestRecheckFreeDiskSpacePrunesOnlyOldFilesWithinRange(t *testing.T) {
	configureCacheTestWithLimit(t, "abcd", "1")
	forceDiskLimitBytes(1)

	oldID := "abcd000000000000000000000000000000000000-4-jpg"
	newID := "abcd111111111111111111111111111111111111-4-jpg"
	oldFile, _ := hvfile.GetHVFileFromFileid(oldID)
	newFile, _ := hvfile.GetHVFileFromFileid(newID)

	writeFileSized(t, oldFile.GetLocalFileRef(), []byte("old!"))
	writeFileSized(t, newFile.GetLocalFileRef(), []byte("new!"))

	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(oldFile.GetLocalFileRef(), oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.Chtimes(newFile.GetLocalFileRef(), newTime, newTime); err != nil {
		t.Fatalf("chtimes new: %v", err)
	}

	ch, err := NewCacheHandler(cacheTestClient{})
	if err != nil {
		t.Fatalf("NewCacheHandler: %v", err)
	}

	if _, err := os.Stat(oldFile.GetLocalFileRef()); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be pruned, got err=%v", err)
	}
	if _, err := os.Stat(newFile.GetLocalFileRef()); err != nil {
		t.Fatalf("expected recent file to remain, got err=%v", err)
	}
	if ch.GetPruneAggression() < 1 {
		t.Fatalf("expected prune aggression to be set")
	}
}
