// Package api 提供 API 功能
package api

import (
	"github.com/qwq/hentaiathomego/internal/network"
)

// API 命令常量
const (
	API_COMMAND_CLIENT_START     = 1
	API_COMMAND_CLIENT_SUSPEND   = 2
	API_COMMAND_MODIFY_SETTING   = 3
	API_COMMAND_REFRESH_SETTINGS = 4
	API_COMMAND_CLIENT_RESUME    = 5
)

// ClientAPIResult API 结果
type ClientAPIResult struct {
	Command int
	Result  string
}

// ClientAPI 客户端 API
type ClientAPI struct {
	client Client
}

// Client 客户端接口
type Client interface {
	SuspendMasterThread(suspendTime int) bool
	ResumeMasterThread() bool
	GetServerHandler() *network.ServerHandler
}

// NewClientAPI 创建新的客户端 API
func NewClientAPI(client Client) *ClientAPI {
	return &ClientAPI{
		client: client,
	}
}

// ClientSuspend 客户端暂停
func (api *ClientAPI) ClientSuspend(suspendTime int) *ClientAPIResult {
	return &ClientAPIResult{
		Command: API_COMMAND_CLIENT_SUSPEND,
		Result:  boolToResult(api.client.SuspendMasterThread(suspendTime)),
	}
}

// ClientResume 客户端恢复
func (api *ClientAPI) ClientResume() *ClientAPIResult {
	return &ClientAPIResult{
		Command: API_COMMAND_CLIENT_RESUME,
		Result:  boolToResult(api.client.ResumeMasterThread()),
	}
}

// RefreshSettings 刷新设置
func (api *ClientAPI) RefreshSettings() *ClientAPIResult {
	success := api.client.GetServerHandler().RefreshServerSettings()
	return &ClientAPIResult{
		Command: API_COMMAND_REFRESH_SETTINGS,
		Result:  boolToResult(success),
	}
}

func boolToResult(b bool) string {
	if b {
		return "OK"
	}
	return "FAIL"
}
