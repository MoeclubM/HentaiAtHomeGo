// Package processors 提供文本响应处理器
package processors

import (
	"unicode/utf8"

	"github.com/qwq/hentaiathomego/internal/util"
)

// HTTPResponseProcessorText 文本响应处理器
type HTTPResponseProcessorText struct {
	HTTPResponseProcessor
	responseBytes []byte
	writeOffset   int
	contentType   string
}

// NewHTTPResponseProcessorText 创建新的文本响应处理器
func NewHTTPResponseProcessorText(responseBody string) *HTTPResponseProcessorText {
	return NewHTTPResponseProcessorTextWithEncoding(responseBody, "text/html", "ISO-8859-1")
}

// NewHTTPResponseProcessorTextWithEncoding 创建带编码的文本响应处理器
func NewHTTPResponseProcessorTextWithEncoding(responseBody, mimeType, charset string) *HTTPResponseProcessorText {
	strLen := utf8.RuneCountInString(responseBody)

	if strLen > 0 {
		util.Debug("响应已写入:")

		if strLen < 10000 {
			util.Debug("%s", responseBody)
		} else {
			util.Debug("太长了")
		}
	}

	// 将字符串转换为字节
	var responseBytes []byte
	if charset == "ISO-8859-1" || charset == "Latin1" {
		// ISO-8859-1 是单字节编码，直接转换
		responseBytes = make([]byte, len(responseBody))
		for i, c := range responseBody {
			responseBytes[i] = byte(c)
		}
	} else {
		// 其他编码使用 UTF-8
		responseBytes = []byte(responseBody)
	}

	contentType := mimeType + "; charset=" + charset

	return &HTTPResponseProcessorText{
		responseBytes: responseBytes,
		writeOffset:   0,
		contentType:   contentType,
	}
}

// GetContentLength 获取内容长度
func (p *HTTPResponseProcessorText) GetContentLength() int {
	if p.responseBytes != nil {
		return len(p.responseBytes)
	}
	return 0
}

// GetContentType 获取内容类型
func (p *HTTPResponseProcessorText) GetContentType() string {
	return p.contentType
}

// GetPreparedTCPBuffer 获取准备好的 TCP 缓冲区
func (p *HTTPResponseProcessorText) GetPreparedTCPBuffer() ([]byte, error) {
	byteCount := util.MinInt(p.GetContentLength()-p.writeOffset, 1460)
	buffer := p.responseBytes[p.writeOffset : p.writeOffset+byteCount]
	p.writeOffset += byteCount

	return buffer, nil
}
