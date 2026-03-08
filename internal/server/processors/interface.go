// Package processors 提供 HTTP 响应处理器接口
package processors

// HTTPResponseProcessorInterface HTTP 响应处理器接口
type HTTPResponseProcessorInterface interface {
	GetContentType() string
	GetContentLength() int
	Initialize() int
	Cleanup()
	GetHeader() string
	AddHeaderField(name, value string)
	RequestCompleted()
	GetPreparedTCPBuffer() ([]byte, error)
}
