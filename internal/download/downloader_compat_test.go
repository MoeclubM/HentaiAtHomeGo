package download

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/pkg/hvfile"
)

type importingCacheHandler struct {
	called       atomic.Bool
	importedTemp string
	importedID   string
	importedData string
}

func (h *importingCacheHandler) ImportFileToCache(tempFile string, hvFile *hvfile.HVFile) bool {
	data, err := os.ReadFile(tempFile)
	if err != nil {
		return false
	}
	h.importedTemp = tempFile
	h.importedID = hvFile.GetFileID()
	h.importedData = string(data)
	h.called.Store(true)
	return true
}

func TestFileDownloaderRetriesOnMissingContentLengthLikeJava(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer server.Close()

	downloader := NewFileDownloader(server.URL, 1000, 1000)
	if err := downloader.DownloadFile(); err == nil {
		t.Fatal("expected missing Content-Length to fail after retries")
	}
	if got := atomic.LoadInt32(&requests); got != 3 {
		t.Fatalf("unexpected retry count: got %d want 3", got)
	}
}

func TestFileDownloaderIgnoresOverallTimeoutLikeJava(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "3")
		if _, err := w.Write([]byte("a")); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(175 * time.Millisecond)
		if _, err := w.Write([]byte("b")); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(175 * time.Millisecond)
		if _, err := w.Write([]byte("c")); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}))
	defer server.Close()

	downloader := NewFileDownloader(server.URL, 500, 250)
	if err := downloader.DownloadFile(); err != nil {
		t.Fatalf("expected download to succeed with only idle timeout enforced: %v", err)
	}
	if downloader.GetContentLength() != 3 {
		t.Fatalf("unexpected content length: got %d want 3", downloader.GetContentLength())
	}
	if downloader.GetDownloadTimeMillis() <= 0 {
		t.Fatalf("expected download time to be recorded, got %d", downloader.GetDownloadTimeMillis())
	}
}

func TestOpenProxySourceAcceptsNon200WhenLengthMatchesLikeJava(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := "x"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	pfd := NewProxyFileDownloader(nil, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-1-jpg", []string{server.URL})
	hv, err := hvfile.GetHVFileFromFileid("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-1-jpg")
	if err != nil {
		t.Fatalf("unexpected hvfile parse error: %v", err)
	}

	resp, cancel, status := pfd.openProxySource(server.URL, hv)
	if status != 200 {
		if cancel != nil {
			cancel()
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("unexpected status: got %d want 200", status)
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		if cancel != nil {
			cancel()
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("unexpected upstream response: %#v", resp)
	}

	pfd.initialResp = resp
	pfd.initialCancel = cancel
	if _, _, _, err := pfd.getProxyBody(server.URL, hv); err == nil {
		t.Fatal("expected non-200 upstream to fail when opening proxy body")
	}
}

func TestProxyFileDownloaderImportsFileAfterProxyCompletion(t *testing.T) {
	body := "hello"
	hash := sha1.Sum([]byte(body))
	fileID := fmt.Sprintf("%s-%d-jpg", hex.EncodeToString(hash[:]), len(body))

	settings := config.GetSettings()
	originalTempDir := settings.GetTempDir()
	tempDir := t.TempDir()
	settings.ParseAndUpdateSettings([]string{"temp_dir=" + tempDir})
	t.Cleanup(func() {
		settings.ParseAndUpdateSettings([]string{"temp_dir=" + originalTempDir})
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cacheHandler := &importingCacheHandler{}
	pfd := NewProxyFileDownloader(cacheHandler, fileID, []string{server.URL})
	if status := pfd.Initialize(); status != 200 {
		t.Fatalf("unexpected initialize status: got %d want 200", status)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !pfd.streamComplete && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !pfd.streamComplete || !pfd.streamSuccess {
		t.Fatalf("expected proxy download to complete successfully: streamComplete=%v streamSuccess=%v", pfd.streamComplete, pfd.streamSuccess)
	}
	if cacheHandler.called.Load() {
		t.Fatal("cache import should wait until proxy request completes")
	}

	tempFile := pfd.tempFile
	pfd.ProxyThreadCompleted()

	deadline = time.Now().Add(5 * time.Second)
	for !pfd.fileFinalized && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !pfd.fileFinalized {
		t.Fatal("expected proxy file finalization after request completion")
	}
	if !cacheHandler.called.Load() {
		t.Fatal("expected successful proxy download to be imported into cache")
	}
	if cacheHandler.importedID != fileID {
		t.Fatalf("unexpected imported file id: got %q want %q", cacheHandler.importedID, fileID)
	}
	if cacheHandler.importedData != body {
		t.Fatalf("unexpected imported file data: got %q want %q", cacheHandler.importedData, body)
	}
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Fatalf("expected temp proxy file to be removed, stat err=%v", err)
	}
}
