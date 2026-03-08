// Package processors 提供文件响应处理器
package processors

import (
	"crypto/sha1"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"strings"
	"time"

	"github.com/qwq/hentaiathomego/internal/cache"
	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/stats"
	"github.com/qwq/hentaiathomego/internal/util"
	"github.com/qwq/hentaiathomego/pkg/hvfile"
)

// HTTPResponseProcessorFile 文件响应处理器
type HTTPResponseProcessorFile struct {
	HTTPResponseProcessor
	requestedHVFile     *hvfile.HVFile
	file                *os.File
	fileOffset          int
	sha1Digest          hash.Hash
	verifyFileIntegrity bool
	cacheHandler        *cache.CacheHandler
	responseStatusCode  int
}

// NewHTTPResponseProcessorFile 创建新的文件响应处理器
func NewHTTPResponseProcessorFile(requestedHVFile *hvfile.HVFile, verifyFileIntegrity bool) *HTTPResponseProcessorFile {
	return &HTTPResponseProcessorFile{
		requestedHVFile:     requestedHVFile,
		verifyFileIntegrity: verifyFileIntegrity,
		responseStatusCode:  0,
	}
}

// SetCacheHandler 设置缓存处理器（用于删除损坏文件）
func (p *HTTPResponseProcessorFile) SetCacheHandler(ch *cache.CacheHandler) {
	p.cacheHandler = ch
}

// Initialize 初始化处理器
func (p *HTTPResponseProcessorFile) Initialize() int {
	if p.verifyFileIntegrity {
		p.sha1Digest = sha1.New()
	}

	filePath := p.requestedHVFile.GetLocalFileRef()
	file, err := os.Open(filePath)
	if err != nil {
		util.Warning("无法读取内容从 %s: %v", filePath, err)
		p.responseStatusCode = 500
		return 500
	}

	p.file = file

	p.responseStatusCode = 200
	stats.FileSent()
	return 200
}

// Cleanup 清理资源
func (p *HTTPResponseProcessorFile) Cleanup() {
	if p.file != nil {
		p.file.Close()
	}

	// 如果启用了完整性验证且文件已完全读取，验证 SHA-1
	if p.sha1Digest != nil && p.fileOffset == p.GetContentLength() {
		calculatedHash := strings.ToLower(hex.EncodeToString(p.sha1Digest.Sum(nil)))
		expectedHash := p.requestedHVFile.GetHash()

		if expectedHash == calculatedHash {
			util.Debug("检查文件完整性 %s，发现预期摘要=%s", p.requestedHVFile.String(), calculatedHash)
		} else {
			util.Warning("检查文件完整性 %s，发现不匹配摘要=%s；损坏的文件将从缓存中删除", p.requestedHVFile.String(), calculatedHash)
			if p.cacheHandler != nil {
				p.cacheHandler.DeleteFileFromCache(p.requestedHVFile)
			}
		}
	}
}

// GetContentType 获取内容类型
func (p *HTTPResponseProcessorFile) GetContentType() string {
	return p.requestedHVFile.GetMimeType()
}

// GetContentLength 获取内容长度
func (p *HTTPResponseProcessorFile) GetContentLength() int {
	if p.file != nil {
		return p.requestedHVFile.GetSize()
	}
	return 0
}

// GetPreparedTCPBuffer 获取准备好的 TCP 缓冲区
func (p *HTTPResponseProcessorFile) GetPreparedTCPBuffer() ([]byte, error) {
	readBytes := util.MinInt(p.GetContentLength()-p.fileOffset, config.TCP_PACKET_SIZE)
	if readBytes <= 0 {
		return []byte{}, nil
	}

	// 直接从文件顺序读取（避免自实现缓冲带来的错位风险）
	tcpBuffer := make([]byte, readBytes)
	readTotal := 0
	for readTotal < readBytes {
		n, err := p.file.Read(tcpBuffer[readTotal:])
		if n > 0 {
			readTotal += n
		}
		if err != nil {
			if err == io.EOF {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return nil, err
		}
		if n == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	p.fileOffset += readBytes

	// 如果启用了完整性验证，更新 SHA-1 摘要
	if p.sha1Digest != nil {
		_, _ = p.sha1Digest.Write(tcpBuffer)
	}

	return tcpBuffer, nil
}
