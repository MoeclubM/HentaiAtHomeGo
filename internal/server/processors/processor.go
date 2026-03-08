// Package processors 提供 HTTP 响应处理器
package processors

// HTTPResponseProcessor HTTP 响应处理器基类
type HTTPResponseProcessor struct {
	header string
}

// GetContentType 获取内容类型
func (p *HTTPResponseProcessor) GetContentType() string {
	return "text/html; charset=iso-8859-1"
}

// GetContentLength 获取内容长度
func (p *HTTPResponseProcessor) GetContentLength() int {
	return 0
}

// Initialize 初始化处理器
func (p *HTTPResponseProcessor) Initialize() int {
	return 0
}

// Cleanup 清理资源
func (p *HTTPResponseProcessor) Cleanup() {
	// 默认不执行任何操作
}

// GetHeader 获取头部
func (p *HTTPResponseProcessor) GetHeader() string {
	return p.header
}

// AddHeaderField 添加头部字段
func (p *HTTPResponseProcessor) AddHeaderField(name, value string) {
	p.header += name + ": " + value + "\r\n"
}

// RequestCompleted 请求完成
func (p *HTTPResponseProcessor) RequestCompleted() {
	// 默认不执行任何操作
}

// GetPreparedTCPBuffer 获取准备好的 TCP 缓冲区
func (p *HTTPResponseProcessor) GetPreparedTCPBuffer() ([]byte, error) {
	return []byte{}, nil
}
