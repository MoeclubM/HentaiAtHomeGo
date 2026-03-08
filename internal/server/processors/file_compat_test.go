package processors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/pkg/hvfile"
)

func TestFileProcessorWaitsForAppendedBytesLikeJava(t *testing.T) {
	settings := config.GetSettings()
	originalCacheDir := settings.GetCacheDir()
	tempCacheDir := t.TempDir()
	settings.ParseAndUpdateSettings([]string{"cache_dir=" + tempCacheDir})
	t.Cleanup(func() {
		settings.ParseAndUpdateSettings([]string{"cache_dir=" + originalCacheDir})
	})

	hv := hvfile.NewHVFile(strings.Repeat("a", 40), 10, 0, 0, "jpg")
	filePath := hv.GetLocalFileRef()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("create cache dirs failed: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write initial file failed: %v", err)
	}

	processor := NewHTTPResponseProcessorFile(hv, false)
	if status := processor.Initialize(); status != 200 {
		t.Fatalf("unexpected initialize status: got %d want 200", status)
	}
	defer processor.Cleanup()

	type result struct {
		chunk []byte
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		chunk, err := processor.GetPreparedTCPBuffer()
		resultCh <- result{chunk: chunk, err: err}
	}()

	select {
	case res := <-resultCh:
		t.Fatalf("processor returned before file was topped up: chunk=%q err=%v", string(res.chunk), res.err)
	case <-time.After(100 * time.Millisecond):
	}

	appendFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open append handle failed: %v", err)
	}
	if _, err := appendFile.Write([]byte("world")); err != nil {
		_ = appendFile.Close()
		t.Fatalf("append remaining bytes failed: %v", err)
	}
	if err := appendFile.Close(); err != nil {
		t.Fatalf("close append handle failed: %v", err)
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("processor returned error after append: %v", res.err)
		}
		if string(res.chunk) != "helloworld" {
			t.Fatalf("unexpected chunk after append: got %q want %q", string(res.chunk), "helloworld")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for processor to read appended bytes")
	}
}
