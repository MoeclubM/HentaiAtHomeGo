// Package network 提供 CakeSphere 存活测试异步处理
package network

import (
	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/stats"
	"github.com/qwq/hentaiathomego/internal/util"
)

// CakeSphere 存活测试异步处理器
type CakeSphere struct {
	handler  *ServerHandler
	client   Client
	doResume bool
}

// NewCakeSphere 创建新的 CakeSphere
func NewCakeSphere(handler *ServerHandler, client Client) *CakeSphere {
	return &CakeSphere{handler: handler, client: client}
}

// StillAlive 执行存活测试（异步）
func (cs *CakeSphere) StillAlive(resume bool) {
	cs.doResume = resume
	go cs.run()
}

func (cs *CakeSphere) run() {
	add := ""
	if cs.doResume {
		add = "resume"
	}

	sr := GetServerResponseWithURL(GetServerConnectionURLWithAdd(ACT_STILL_ALIVE, add), cs.handler)

	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		util.Debug("成功对服务器执行了存活测试")
		stats.ServerContact()
		return
	}

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		config.GetSettings().MarkRPCServerFailure(sr.GetFailHost())
		util.Warning("无法连接到服务器进行存活测试。这可能是临时连接问题。")
		return
	}

	if sr.GetFailCode() == "TERM_BAD_NETWORK" {
		cs.client.DieWithError("客户端正在关闭，因为网络配置错误；请更正防火墙/转发设置然后重启客户端。")
		return
	}

	util.Warning("存活测试失败: (%s) - 稍后重试", sr.GetFailCode())
}
