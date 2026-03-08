package config

import (
	"bufio"
	"fmt"
	"os"

	"github.com/qwq/hentaiathomego/internal/util"
)

// InputQueryHandlerCLI CLI 输入查询处理器
// 实现 util.InputQueryHandler
// 用于首次启动时输入 Client ID/Key
type InputQueryHandlerCLI struct {
	scanner *bufio.Scanner
}

// NewInputQueryHandlerCLI 创建新的 CLI 输入查询处理器
func NewInputQueryHandlerCLI() (*InputQueryHandlerCLI, error) {
	return &InputQueryHandlerCLI{
		scanner: bufio.NewScanner(os.Stdin),
	}, nil
}

var _ util.InputQueryHandler = (*InputQueryHandlerCLI)(nil)

// QueryString 查询字符串输入
func (h *InputQueryHandlerCLI) QueryString(queryText string) (string, error) {
	fmt.Printf("%s: ", queryText)

	if h.scanner.Scan() {
		return h.scanner.Text(), nil
	}

	if err := h.scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("输入被中断")
}
