package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/qwq/hentaiathomego/internal/util"
)

// InputQueryHandlerCLI CLI 输入查询处理器
// 实现 util.InputQueryHandler
// 用于首次启动时输入 Client ID/Key
type InputQueryHandlerCLI struct {
	scanner          *bufio.Scanner
	envClientID      string
	envClientKey     string
	envClientIDUsed  bool
	envClientKeyUsed bool
}

// NewInputQueryHandlerCLI 创建新的 CLI 输入查询处理器
func NewInputQueryHandlerCLI() (*InputQueryHandlerCLI, error) {
	return &InputQueryHandlerCLI{
		scanner:      bufio.NewScanner(os.Stdin),
		envClientID:  firstNonEmptyEnv("HATH_CLIENT_ID", "HATHGO_CLIENT_ID"),
		envClientKey: firstNonEmptyEnv("HATH_CLIENT_KEY", "HATHGO_CLIENT_KEY"),
	}, nil
}

var _ util.InputQueryHandler = (*InputQueryHandlerCLI)(nil)

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func (h *InputQueryHandlerCLI) takePrefilledValue(queryText string) (string, bool) {
	normalized := strings.ToLower(queryText)
	if !h.envClientIDUsed && h.envClientID != "" && strings.Contains(normalized, "id") {
		h.envClientIDUsed = true
		return h.envClientID, true
	}
	if !h.envClientKeyUsed && h.envClientKey != "" && (strings.Contains(normalized, "key") || strings.Contains(queryText, "密钥") || strings.Contains(queryText, "密鑰")) {
		h.envClientKeyUsed = true
		return h.envClientKey, true
	}
	return "", false
}

// QueryString 查询字符串输入
func (h *InputQueryHandlerCLI) QueryString(queryText string) (string, error) {
	if value, ok := h.takePrefilledValue(queryText); ok {
		return value, nil
	}

	fmt.Printf("%s: ", queryText)

	if h.scanner.Scan() {
		return h.scanner.Text(), nil
	}

	if err := h.scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("interactive input unavailable; create data/%s or set HATH_CLIENT_ID/HATH_CLIENT_KEY", CLIENT_LOGIN_FILENAME)
}
