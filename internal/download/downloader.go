package download

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/stats"
	"github.com/qwq/hentaiathomego/internal/util"
	"github.com/qwq/hentaiathomego/pkg/hvfile"
	xproxy "golang.org/x/net/proxy"
)

type FileDownloader struct {
	source          string
	timeout         time.Duration
	maxDLTime       time.Duration
	outputPath      string
	allowProxy      bool
	discardData     bool
	contentLength   int
	downloadTimeMS  int64
	successful      bool
	downloadLimiter interface{}
}

func NewFileDownloader(source string, timeout, maxDLTime int) *FileDownloader {
	return &FileDownloader{
		source:      source,
		timeout:     time.Duration(timeout) * time.Millisecond,
		maxDLTime:   time.Duration(maxDLTime) * time.Millisecond,
		discardData: true,
	}
}

func NewFileDownloaderWithOutput(source string, timeout, maxDLTime int, outputPath string, allowProxy bool) *FileDownloader {
	return &FileDownloader{
		source:      source,
		timeout:     time.Duration(timeout) * time.Millisecond,
		maxDLTime:   time.Duration(maxDLTime) * time.Millisecond,
		outputPath:  outputPath,
		allowProxy:  allowProxy,
		discardData: outputPath == "",
	}
}

func (fd *FileDownloader) DownloadFile() error {
	return fd.downloadFile()
}

func (fd *FileDownloader) downloadFile() error {
	settings := config.GetSettings()
	fd.successful = false

	retries := 3
	for retries > 0 {
		retries--

		util.Debug("连接到 %s...", fd.source)

		resp, cancel, err := openGET(fd.source, fd.timeout, fd.allowProxy, func(req *http.Request) {
			req.Header.Set("Connection", "Close")
			req.Header.Set("User-Agent", "Hentai@Home "+config.CLIENT_VERSION)
		})
		if err != nil {
			util.Warning("请求失败: %v", err)
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			cancel()
			util.Warning("服务器返回: 404 Not Found")
			return fmt.Errorf("file not found")
		}

		fd.contentLength = int(resp.ContentLength)
		if fd.contentLength < 0 {
			resp.Body.Close()
			cancel()
			util.Warning("请求主机未发送 Content-Length")
			continue
		}
		if !fd.discardData && fd.outputPath == "" && fd.contentLength > 10485760 {
			resp.Body.Close()
			cancel()
			util.Warning("报告的内容长度 %d 超过内存缓冲下载上限", fd.contentLength)
			continue
		}
		if fd.contentLength > settings.GetMaxAllowedFileSize() {
			resp.Body.Close()
			cancel()
			util.Warning("报告的内容长度 %d 超过最大允许文件大小 %d", fd.contentLength, settings.GetMaxAllowedFileSize())
			continue
		}

		util.Debug("读取 %d 字节从 %s", fd.contentLength, fd.source)

		writtenBytes, readErr := fd.consumeResponse(resp.Body, cancel)
		resp.Body.Close()
		cancel()

		if readErr != nil {
			util.Warning("读取响应失败: %v", readErr)
			if fd.outputPath != "" {
				_ = os.Remove(fd.outputPath)
			}
			continue
		}
		if int(writtenBytes) != fd.contentLength {
			util.Warning("下载不完整: %d/%d 字节", writtenBytes, fd.contentLength)
			if fd.outputPath != "" {
				_ = os.Remove(fd.outputPath)
			}
			continue
		}

		fd.successful = true
		stats.BytesRcvd(fd.contentLength)
		util.Debug("完成下载 %s，大小 %d 字节", fd.source, fd.contentLength)
		return nil
	}

	return fmt.Errorf("download failed: retries exhausted")
}

func (fd *FileDownloader) consumeResponse(body io.Reader, abort context.CancelFunc) (int64, error) {
	fd.downloadTimeMS = 0
	firstByteTime := time.Time{}
	wrapChunk := func(writer func([]byte) error) func([]byte) error {
		return func(chunk []byte) error {
			if len(chunk) > 0 && firstByteTime.IsZero() {
				firstByteTime = time.Now()
			}
			return writer(chunk)
		}
	}

	var (
		written int64
		err     error
	)

	if fd.outputPath != "" && !fd.discardData {
		if err := os.MkdirAll(filepath.Dir(fd.outputPath), 0755); err != nil {
			return 0, err
		}

		file, err := os.Create(fd.outputPath)
		if err != nil {
			return 0, err
		}
		defer file.Close()

		written, err = copyWithTimeouts(body, fd.timeout, 0, abort, wrapChunk(func(chunk []byte) error {
			_, writeErr := file.Write(chunk)
			return writeErr
		}))
	} else {
		written, err = copyWithTimeouts(body, fd.timeout, 0, abort, wrapChunk(func(chunk []byte) error {
			_, writeErr := io.Discard.Write(chunk)
			return writeErr
		}))
	}

	if !firstByteTime.IsZero() {
		fd.downloadTimeMS = time.Since(firstByteTime).Milliseconds()
	}

	return written, err
}

func (fd *FileDownloader) GetContentLength() int {
	return fd.contentLength
}

func (fd *FileDownloader) GetDownloadTimeMillis() int64 {
	return fd.downloadTimeMS
}

func (fd *FileDownloader) IsSuccessful() bool {
	return fd.successful
}

type ProxyFileDownloader struct {
	cacheHandler   CacheHandler
	fileID         string
	sources        []string
	selectedSource string
	tempFile       string
	contentLength  int
	readOffset     int
	writeOffset    int
	streamSuccess  bool
	streamComplete bool
	proxyComplete  bool
	fileFinalized  bool
	initialResp    *http.Response
	initialCancel  context.CancelFunc
}

type CacheHandler interface {
	ImportFileToCache(tempFile string, hvFile *hvfile.HVFile) bool
}

func NewProxyFileDownloader(cacheHandler CacheHandler, fileID string, sources []string) *ProxyFileDownloader {
	return &ProxyFileDownloader{
		cacheHandler: cacheHandler,
		fileID:       fileID,
		sources:      sources,
	}
}

func (pfd *ProxyFileDownloader) Initialize() int {
	hvFile, err := hvfile.GetHVFileFromFileid(pfd.fileID)
	if err != nil {
		return 500
	}

	retval := 500
	for _, source := range pfd.sources {
		resp, cancel, statusCode := pfd.openProxySource(source, hvFile)
		if statusCode != 200 {
			if statusCode > retval {
				retval = statusCode
			}
			continue
		}

		settings := config.GetSettings()
		tempFile := filepath.Join(settings.GetTempDir(), "proxyfile_"+strconv.FormatInt(time.Now().UnixNano(), 10)+".tmp")
		file, fileErr := os.Create(tempFile)
		if fileErr != nil {
			resp.Body.Close()
			cancel()
			util.Warning("创建临时文件失败: %v", fileErr)
			return 500
		}
		file.Close()

		pfd.selectedSource = source
		pfd.initialResp = resp
		pfd.initialCancel = cancel
		pfd.tempFile = tempFile
		pfd.contentLength = hvFile.GetSize()

		go pfd.run()
		return 200
	}

	return retval
}

func (pfd *ProxyFileDownloader) run() {
	hvFile, err := hvfile.GetHVFileFromFileid(pfd.fileID)
	if err == nil {
		tryCounter := 3
		for !pfd.streamSuccess && tryCounter > 0 {
			if pfd.downloadFromSource(pfd.selectedSource, hvFile) {
				pfd.streamSuccess = true
				break
			}
			tryCounter--
			pfd.writeOffset = 0
		}
	}

	pfd.streamComplete = true
	pfd.checkFinalizeDownloadedFile()
}

func (pfd *ProxyFileDownloader) downloadFromSource(source string, hvFile *hvfile.HVFile) bool {
	file, err := os.OpenFile(pfd.tempFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		util.Warning("打开临时文件失败: %v", err)
		return false
	}
	defer file.Close()

	body, abort, closeBody, err := pfd.getProxyBody(source, hvFile)
	if err != nil {
		util.Warning("下载失败: %v", err)
		return false
	}
	defer func() {
		closeBody()
		abort()
	}()

	writeOffset := 0
	hasher := sha1.New()

	writtenBytes, readErr := copyWithTimeouts(body, 30*time.Second, 300*time.Second, abort, func(chunk []byte) error {
		if _, err := hasher.Write(chunk); err != nil {
			return err
		}
		if _, err := file.WriteAt(chunk, int64(writeOffset)); err != nil {
			return err
		}
		writeOffset += len(chunk)
		pfd.writeOffset = writeOffset
		stats.BytesRcvd(len(chunk))
		return nil
	})
	if readErr != nil {
		util.Warning("读取响应失败: %v", readErr)
		return false
	}
	if int(writtenBytes) != pfd.contentLength {
		util.Warning("下载不完整: %d/%d 字节", writtenBytes, pfd.contentLength)
		return false
	}

	calculatedHash := strings.ToLower(hex.EncodeToString(hasher.Sum(nil)))
	if calculatedHash != hvFile.GetHash() {
		util.Warning("SHA-1 不匹配: 期望 %s，得到 %s", hvFile.GetHash(), calculatedHash)
		return false
	}

	return true
}

func (pfd *ProxyFileDownloader) checkFinalizeDownloadedFile() {
	if !pfd.streamComplete || !pfd.proxyComplete {
		return
	}
	if pfd.fileFinalized {
		return
	}

	pfd.fileFinalized = true

	hvFile, err := hvfile.GetHVFileFromFileid(pfd.fileID)
	if err == nil {
		if fileInfo, statErr := os.Stat(pfd.tempFile); statErr == nil {
			if pfd.streamSuccess && fileInfo.Size() == int64(pfd.contentLength) && pfd.cacheHandler != nil && pfd.cacheHandler.ImportFileToCache(pfd.tempFile, hvFile) {
				util.Debug("代理下载文件 %s 已成功存储到缓存", pfd.fileID)
			}
		}
	}

	_ = os.Remove(pfd.tempFile)
}

func (pfd *ProxyFileDownloader) GetContentType() string {
	hvFile, _ := hvfile.GetHVFileFromFileid(pfd.fileID)
	return hvFile.GetMimeType()
}

func (pfd *ProxyFileDownloader) GetContentLength() int {
	return pfd.contentLength
}

func (pfd *ProxyFileDownloader) GetCurrentWriteOffset() int {
	return pfd.writeOffset
}

func (pfd *ProxyFileDownloader) FillBuffer(buffer []byte, offset int) (int, error) {
	file, err := os.Open(pfd.tempFile)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	bytesToRead := minInt(len(buffer), pfd.writeOffset-offset)
	if bytesToRead <= 0 {
		return 0, nil
	}

	n, err := file.ReadAt(buffer[:bytesToRead], int64(offset))
	if err == io.EOF && n > 0 {
		return n, nil
	}
	return n, err
}

func (pfd *ProxyFileDownloader) ProxyThreadCompleted() {
	stats.FileSent()
	pfd.proxyComplete = true
	pfd.checkFinalizeDownloadedFile()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type readResult struct {
	n   int
	err error
}

func copyWithTimeouts(body io.Reader, idleTimeout, totalTimeout time.Duration, abort context.CancelFunc, onChunk func([]byte) error) (int64, error) {
	startTime := time.Now()
	var total int64

	for {
		waitTimeout := idleTimeout
		if totalTimeout > 0 {
			remaining := totalTimeout - time.Since(startTime)
			if remaining <= 0 {
				abort()
				return total, fmt.Errorf("Download timed out")
			}
			if waitTimeout <= 0 || remaining < waitTimeout {
				waitTimeout = remaining
			}
		}
		if waitTimeout <= 0 {
			waitTimeout = 24 * time.Hour
		}

		buffer := make([]byte, 65536)
		readCh := make(chan readResult, 1)

		go func(readBuffer []byte) {
			n, err := body.Read(readBuffer)
			readCh <- readResult{n: n, err: err}
		}(buffer)

		select {
		case result := <-readCh:
			if result.n > 0 {
				if err := onChunk(buffer[:result.n]); err != nil {
					abort()
					return total, err
				}
				total += int64(result.n)
			}

			if result.err == io.EOF {
				return total, nil
			}
			if result.err != nil {
				return total, result.err
			}
		case <-time.After(waitTimeout):
			abort()
			return total, fmt.Errorf("Read timed out")
		}
	}
}

func newHTTPClient(headerTimeout time.Duration, allowProxy bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: headerTimeout,
	}

	settings := config.GetSettings()
	if allowProxy && settings.IsImageProxyEnabled() {
		switch strings.ToLower(settings.GetImageProxyType()) {
		case "http":
			proxyURL, err := url.Parse(fmt.Sprintf("http://%s:%d", settings.GetImageProxyHost(), settings.GetImageProxyPort()))
			if err == nil {
				transport.Proxy = http.ProxyURL(proxyURL)
			}
		case "socks":
			socksDialer, err := xproxy.SOCKS5("tcp", fmt.Sprintf("%s:%d", settings.GetImageProxyHost(), settings.GetImageProxyPort()), nil, dialer)
			if err == nil {
				transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					return socksDialer.Dial(network, address)
				}
			}
		}
	}

	return &http.Client{Transport: transport}
}

func openGET(source string, headerTimeout time.Duration, allowProxy bool, prepare func(*http.Request)) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", source, nil)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	if prepare != nil {
		prepare(req)
	}

	resp, err := newHTTPClient(headerTimeout, allowProxy).Do(req)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}

	return resp, cancel, nil
}

func (pfd *ProxyFileDownloader) openProxySource(source string, hvFile *hvfile.HVFile) (*http.Response, context.CancelFunc, int) {
	resp, cancel, err := openGET(source, 30*time.Second, true, func(req *http.Request) {
		settings := config.GetSettings()
		req.Header.Set("Hath-Request", fmt.Sprintf("%d-%s", settings.GetClientID(), util.GetSHA1String(settings.GetClientKey()+pfd.fileID)))
		req.Header.Set("User-Agent", "Hentai@Home "+config.CLIENT_VERSION)
	})
	if err != nil {
		util.Warning("下载失败: %v", err)
		return nil, func() {}, 500
	}

	if false && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		util.Warning("服务器返回状态码: %d", resp.StatusCode)
		return nil, func() {}, 502
	}

	contentLength := int(resp.ContentLength)
	if contentLength < 0 {
		resp.Body.Close()
		cancel()
		util.Warning("请求主机未发送 Content-Length")
		return nil, func() {}, 502
	}
	if contentLength > config.GetSettings().GetMaxAllowedFileSize() {
		resp.Body.Close()
		cancel()
		util.Warning("报告的内容长度 %d 超过最大允许文件大小 %d", contentLength, config.GetSettings().GetMaxAllowedFileSize())
		return nil, func() {}, 502
	}
	if contentLength != hvFile.GetSize() {
		resp.Body.Close()
		cancel()
		util.Warning("报告的内容长度 %d 与预期文件长度 %d 不匹配", contentLength, hvFile.GetSize())
		return nil, func() {}, 502
	}

	return resp, cancel, 200
}

func (pfd *ProxyFileDownloader) getProxyBody(source string, hvFile *hvfile.HVFile) (io.Reader, context.CancelFunc, func(), error) {
	if pfd.initialResp != nil {
		resp := pfd.initialResp
		cancel := pfd.initialCancel
		pfd.initialResp = nil
		pfd.initialCancel = nil
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			cancel()
			return nil, func() {}, func() {}, fmt.Errorf("upstream returned status %d", resp.StatusCode)
		}
		return resp.Body, cancel, func() { _ = resp.Body.Close() }, nil
	}

	resp, cancel, statusCode := pfd.openProxySource(source, hvFile)
	if statusCode != 200 {
		return nil, func() {}, func() {}, fmt.Errorf("upstream returned status %d", statusCode)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		return nil, func() {}, func() {}, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	return resp.Body, cancel, func() { _ = resp.Body.Close() }, nil
}
