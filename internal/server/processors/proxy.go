// Package processors 提供代理响应处理器
package processors

import (
	"errors"
	"time"

	"github.com/qwq/hentaiathomego/internal/util"
)

// HTTPResponseProcessorProxy 代理响应处理器
type HTTPResponseProcessorProxy struct {
	HTTPResponseProcessor
	fileID          string
	proxyDownloader ProxyDownloader
	readOffset      int
	tcpBuffer       []byte
}

// ProxyDownloader 代理下载器接口
type ProxyDownloader interface {
	Initialize() int
	GetContentType() string
	GetContentLength() int
	GetCurrentWriteOffset() int
	FillBuffer(buffer []byte, offset int) (int, error)
	ProxyThreadCompleted()
}

// NewHTTPResponseProcessorProxy 创建新的代理响应处理器
func NewHTTPResponseProcessorProxy(fileID string) *HTTPResponseProcessorProxy {
	return &HTTPResponseProcessorProxy{
		fileID:     fileID,
		tcpBuffer:  make([]byte, 1460),
		readOffset: 0,
	}
}

// SetProxyDownloader 设置代理下载器
func (p *HTTPResponseProcessorProxy) SetProxyDownloader(downloader ProxyDownloader) {
	p.proxyDownloader = downloader
}

// Initialize 初始化处理器
func (p *HTTPResponseProcessorProxy) Initialize() int {
	util.Debug("会话: 初始化代理请求...")
	return p.proxyDownloader.Initialize()
}

// GetContentType 获取内容类型
func (p *HTTPResponseProcessorProxy) GetContentType() string {
	return p.proxyDownloader.GetContentType()
}

// GetContentLength 获取内容长度
func (p *HTTPResponseProcessorProxy) GetContentLength() int {
	return p.proxyDownloader.GetContentLength()
}

// GetPreparedTCPBuffer 获取准备好的 TCP 缓冲区
func (p *HTTPResponseProcessorProxy) GetPreparedTCPBuffer() ([]byte, error) {
	p.tcpBuffer = make([]byte, 1460)

	timeout := 0
	nextReadThreshold := util.MinInt(p.GetContentLength(), p.readOffset+len(p.tcpBuffer))

	// 等待代理下载器有足够的数据
	for nextReadThreshold > p.proxyDownloader.GetCurrentWriteOffset() {
		time.Sleep(10 * time.Millisecond)

		if timeout++; timeout > 30000 {
			return nil, errors.New("Timeout while waiting for proxy request.")
		}
	}

	readBytes, err := p.proxyDownloader.FillBuffer(p.tcpBuffer, p.readOffset)
	if err != nil {
		return nil, err
	}

	p.readOffset += readBytes

	return p.tcpBuffer[:readBytes], nil
}

// RequestCompleted 请求完成
func (p *HTTPResponseProcessorProxy) RequestCompleted() {
	p.proxyDownloader.ProxyThreadCompleted()
}
