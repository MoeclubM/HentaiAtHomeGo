// Package processors 提供速度测试响应处理器
package processors

import (
	"crypto/rand"
	"math/big"

	"github.com/qwq/hentaiathomego/internal/util"
)

// HTTPResponseProcessorSpeedtest 速度测试响应处理器
type HTTPResponseProcessorSpeedtest struct {
	HTTPResponseProcessor
	testSize    int
	writeOffset int
	randomBytes []byte
}

// NewHTTPResponseProcessorSpeedtest 创建新的速度测试处理器
func NewHTTPResponseProcessorSpeedtest(testSize int) *HTTPResponseProcessorSpeedtest {
	randomLength := 8192
	randomBytes := make([]byte, randomLength)

	// 生成随机字节
	for i := range randomBytes {
		n, _ := rand.Int(rand.Reader, big.NewInt(256))
		randomBytes[i] = byte(n.Int64())
	}

	return &HTTPResponseProcessorSpeedtest{
		testSize:    testSize,
		writeOffset: 0,
		randomBytes: randomBytes,
	}
}

// GetContentLength 获取内容长度
func (p *HTTPResponseProcessorSpeedtest) GetContentLength() int {
	return p.testSize
}

// GetPreparedTCPBuffer 获取准备好的 TCP 缓冲区
func (p *HTTPResponseProcessorSpeedtest) GetPreparedTCPBuffer() ([]byte, error) {
	byteCount := util.MinInt(p.GetContentLength()-p.writeOffset, 1460)

	// 随机选择起始位置
	startByte, _ := rand.Int(rand.Reader, big.NewInt(int64(len(p.randomBytes)-byteCount)))
	start := int(startByte.Int64())

	buffer := make([]byte, byteCount)
	copy(buffer, p.randomBytes[start:start+byteCount])
	p.writeOffset += byteCount

	return buffer, nil
}
