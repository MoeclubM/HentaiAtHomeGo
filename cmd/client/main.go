// Package main 提供主程序入口
package main

import (
	"strconv"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qwq/hentaiathomego/internal/api"
	"github.com/qwq/hentaiathomego/internal/cache"
	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/network"
	"github.com/qwq/hentaiathomego/internal/server"
	"github.com/qwq/hentaiathomego/internal/stats"
	"github.com/qwq/hentaiathomego/internal/util"
)

// HentaiAtHomeClient H@H 客户端
type HentaiAtHomeClient struct {
	serverHandler    *network.ServerHandler
	httpServer       *server.Server
	cacheHandler     *cache.CacheHandler
	clientAPI        *api.ClientAPI
	shutdown         bool
	suspendedUntil   int64
	threadSkipCounter int
	doCertRefresh    bool
	iqh              *config.InputQueryHandlerCLI
}

func main() {
	// 解析命令行参数
	args := os.Args[1:]

	// 创建输入查询处理器
	iqh, err := config.NewInputQueryHandlerCLI()
	if err != nil {
		util.Error("无法初始化输入查询处理器: %v", err)
		os.Exit(-1)
	}

	client := &HentaiAtHomeClient{
		suspendedUntil:   0,
		threadSkipCounter: 1,
		iqh:             iqh,
	}

	client.run(iqh, args)
}

// run 运行客户端
func (c *HentaiAtHomeClient) run(iqh *config.InputQueryHandlerCLI, args []string) {
	stats.SetProgramStatus("初始化...")

	// 设置系统属性
	// TODO: 设置 HTTP keep-alive

	// 设置活动客户端
	config.GetSettings() // 初始化设置

	// 解析命令行参数
	settings := config.GetSettings()
	settings.ParseArgs(args)

	// 初始化目录
	if err := settings.InitializeDirectories(); err != nil {
		util.Error("无法创建程序目录。检查文件访问权限和可用磁盘空间。")
		os.Exit(-1)
	}

	// 启动日志系统
	out := util.GetOut()
	if err := out.StartLoggers(settings.GetLogDir()); err != nil {
		util.Error("无法启动日志系统: %v", err)
		os.Exit(-1)
	}

	util.Info("Hentai@Home %s (Build %d) 启动中\n", config.CLIENT_VERSION, config.CLIENT_BUILD)
	util.Info("Copyright (c) 2008-2024, E-Hentai.org - all rights reserved.")
	util.Info("本软件附带绝对没有任何保证。这是免费软件，欢迎您在 GPL v3 许可证下修改和重新分发。\n")

	stats.ResetStats()
	stats.SetProgramStatus("登录到主服务器...")

	// 创建客户端 API
	c.clientAPI = api.NewClientAPI(c)

	// 加载客户端登录信息
	settings.LoadClientLoginFromFile()

	// 如果登录凭证无效，提示输入
	if !settings.LoginCredentialsAreSyntaxValid() {
		if err := settings.PromptForIDAndKey(iqh); err != nil {
			util.Error("输入客户端凭证失败: %v", err)
			os.Exit(-1)
		}
	}

	// 创建服务器处理器
	c.serverHandler = network.NewServerHandler(c)

	// 从服务器加载客户端设置
	stats.SetProgramStatus("正在从服务器加载设置...")
	c.serverHandler.LoadClientSettingsFromServer()

	// 检查是否需要退出
	if c.shutdown {
		return
	}

	// 初始化缓存处理
	stats.SetProgramStatus("正在初始化缓存处理...")
	cacheHandler, err := cache.NewCacheHandler(c)
	if err != nil {
		c.DieWithError(err.Error())
		return
	}
	c.cacheHandler = cacheHandler

	if c.shutdown {
		return
	}

	// 添加关闭钩子
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		util.Info("接收到中断信号，正在关闭...")
		c.shutdown = true
	}()

	// 启动 HTTP 服务器
	stats.SetProgramStatus("正在启动 HTTP 服务器...")
	c.httpServer = createServer(c)

	port := settings.GetClientPort()
	if err := c.httpServer.StartConnectionListener(port); err != nil {
		c.DieWithError("无法初始化 HTTP 服务器")
		return
	}

	stats.SetProgramStatus("正在发送启动通知...")

	util.Info("通知服务器客户端启动已完成...")

	if !c.serverHandler.NotifyStart() {
		util.Info("启动通知失败。")
		return
	}

	c.httpServer.AllowNormalConnections()

	// 检查是否有新版本
	if settings.IsWarnNewClient() {
		util.Warning("有新版本可用。请从 http://hentaiathome.net/ 下载")
	}

	if cacheHandler.GetCacheCount() < 1 {
		util.Info("重要：您的缓存尚未包含任何文件。暂时不会看到任何流量。")
		util.Info("对于新客户端，可能需要几天到几周时间才能看到显著流量。")
	}

	// 刷新服务器设置
	c.serverHandler.RefreshServerSettings()

	stats.ResetBytesSentHistory()
	stats.ProgramStarted()

	// 处理黑名单
	cacheHandler.ProcessBlacklist(259200)

	c.suspendedUntil = 0
	c.threadSkipCounter = 1

	util.Info("启动成功完成，开始正常运行")

	// 主循环
	c.mainLoop(cacheHandler)
}

// mainLoop 主循环
func (c *HentaiAtHomeClient) mainLoop(cacheHandler *cache.CacheHandler) {
	lastThreadTime := int64(0)

	for !c.shutdown {
		// 计算睡眠时间
		sleeptime := int64(1000)
		if lastThreadTime > 0 {
			sleeptime = int64(10000) - (time.Now().UnixMilli() - lastThreadTime)
			if sleeptime < 1000 {
				sleeptime = 1000
			}
			if sleeptime > 10000 {
				sleeptime = 10000
			}
		}

		time.Sleep(time.Duration(sleeptime) * time.Millisecond)

		// 检查是否暂停
		if !c.shutdown && c.suspendedUntil < time.Now().UnixMilli() {
			util.Debug("主线程周期开始")

			if c.suspendedUntil > 0 {
				c.resumeMasterThread()
			}

			// 每 11 个周期执行存活测试
			if c.threadSkipCounter%11 == 0 {
				c.serverHandler.StillAliveTest(false)
			}

			// 每 30 个周期检查时间
			if c.threadSkipCounter%30 == 1 {
				if abs(config.GetSettings().GetServerTimeDelta()) > 86400 {
					util.Warning("系统时间偏差超过 24 小时。请更正系统时间。")
				}

				if c.httpServer.IsCertExpired() {
					c.dieWithError("系统时间可能严重错误，或证书续期失败。请检查系统时间和网络后手动重启客户端。")
					return
				}
			}

			// 每 6 个周期清理洪水控制表
			if c.threadSkipCounter%6 == 2 {
				c.httpServer.PruneFloodControlTable()
			}

			// 每 1440 个周期清理 RPC 服务器失败记录
			if c.threadSkipCounter%1440 == 1439 {
				config.GetSettings().ClearRPCServerFailure()
			}

			// 每 2160 个周期处理黑名单
			if c.threadSkipCounter%2160 == 2159 {
				cacheHandler.ProcessBlacklist(43200)
			}

			// 循环 LRU 缓存表
			cacheHandler.CycleLRUCacheTable()

			// 清理旧连接
			c.httpServer.NukeOldConnections()

			// 移动发送字节历史
			stats.ShiftBytesSentHistory()

			// 检查磁盘空间
			for i := 0; i < cacheHandler.GetPruneAggression(); i++ {
				if !cacheHandler.RecheckFreeDiskSpace() {
					c.dieWithError("可用磁盘空间低于最小阈值。请释放磁盘空间或从设置页面减少缓存大小。")
					return
				}
			}

			if c.doCertRefresh {
				c.handleCertRefresh()
			}

			c.threadSkipCounter++
		}

		lastThreadTime = time.Now().UnixMilli()
	}

	c.shutdownCleanup(cacheHandler)
}

// shutdownCleanup 关闭清理
func (c *HentaiAtHomeClient) shutdownCleanup(cacheHandler *cache.CacheHandler) {
	util.Info("正在关闭...")

	if !c.shutdown {
		c.shutdown = true
		c.serverHandler.NotifyShutdown()
	}

	// 等待连接关闭
	time.Sleep(5 * time.Second)

	if cacheHandler != nil {
		cacheHandler.TerminateCache()
	}

	util.Info("关闭完成。")
}

// suspendMasterThread 暂停主线程
// SuspendMasterThread 暂停主线程（对外接口）
func (c *HentaiAtHomeClient) SuspendMasterThread(suspendTime int) bool {
	if suspendTime > 0 && suspendTime <= 86400 && c.suspendedUntil < time.Now().UnixMilli() {
		stats.ProgramSuspended()
		c.suspendedUntil = time.Now().UnixMilli() + int64(suspendTime)*1000
		return c.serverHandler.NotifySuspend()
	}
	return false
}

// ResumeMasterThread 恢复主线程（对外接口）
func (c *HentaiAtHomeClient) ResumeMasterThread() bool {
	c.suspendedUntil = 0
	c.threadSkipCounter = 0
	stats.ProgramResumed()
	return c.serverHandler.NotifyResume()
}

// DieWithError 因错误退出（对外接口）
func (c *HentaiAtHomeClient) DieWithError(error string) {
	util.Error("严重错误: %s", error)
	stats.SetProgramStatus("已死亡")
	c.shutdown = true
}

// 内部别名，保留旧调用点
func (c *HentaiAtHomeClient) suspendMasterThread(suspendTime int) bool { return c.SuspendMasterThread(suspendTime) }
func (c *HentaiAtHomeClient) resumeMasterThread() bool               { return c.ResumeMasterThread() }
func (c *HentaiAtHomeClient) dieWithError(error string)              { c.DieWithError(error) }

// GetServerHandler 获取服务器处理器
func (c *HentaiAtHomeClient) GetServerHandler() *network.ServerHandler {
	return c.serverHandler
}

func (c *HentaiAtHomeClient) SetCertRefresh() {
	c.doCertRefresh = true
}

func (c *HentaiAtHomeClient) StartDownloader() {
	c.startDownloader()
}

func (c *HentaiAtHomeClient) handleCertRefresh() {
	util.Info("内部重启 HTTP 服务器以刷新证书")

	if !c.serverHandler.NotifySuspend() {
		util.Warning("无法通知服务器暂停流量，将稍后重试证书刷新")
		return
	}

	time.Sleep(5 * time.Second)
	c.httpServer.StopConnectionListener(true)

	for i := 0; i < 60 && !c.httpServer.IsThreadTerminated(); i++ {
		util.Info("等待 HTTP 服务器线程终止...%s", func() string {
			if i > 0 {
				return " (已等待 " + fmtInt((i+1)*5) + " 秒)"
			}
			return ""
		}())
		time.Sleep(5 * time.Second)
	}

	time.Sleep(1 * time.Second)
	newServer := createServer(c)
	if err := newServer.StartConnectionListener(config.GetSettings().GetClientPort()); err != nil {
		c.DieWithError("证书刷新后无法重新初始化 HTTP 服务器")
		return
	}

	c.httpServer = newServer
	c.httpServer.AllowNormalConnections()
	c.serverHandler.StillAliveTest(true)
	c.doCertRefresh = false

	util.Info("内部 HTTP 服务器已完成重启")
}

func fmtInt(v int) string {
	return strconv.Itoa(v)
}

// GetCacheHandler 获取缓存处理器
func (c *HentaiAtHomeClient) GetCacheHandler() *cache.CacheHandler {
	return c.cacheHandler
}

func (c *HentaiAtHomeClient) NotifyOverload() {
	// 由 server 层触发，转发给 serverHandler（带节流）
	if c.serverHandler != nil {
		c.serverHandler.NotifyOverload()
	}
}

// IsShuttingDown 检查是否正在关闭
func (c *HentaiAtHomeClient) IsShuttingDown() bool {
	return c.shutdown
}

// GetInputQueryHandler 获取输入查询处理器
func (c *HentaiAtHomeClient) GetInputQueryHandler() util.InputQueryHandler {
	return c.iqh
}

func (c *HentaiAtHomeClient) PromptForIDAndKey() {
	if c.iqh == nil {
		return
	}
	if err := config.GetSettings().PromptForIDAndKey(c.iqh); err != nil {
		util.Warning("重新输入客户端凭证失败: %v", err)
	}
}

// startDownloader 启动下载器
func (c *HentaiAtHomeClient) startDownloader() {
	// TODO: 实现下载器
}

// deleteDownloader 删除下载器
func (c *HentaiAtHomeClient) deleteDownloader() {
	// TODO: 实现下载器
}

// createServer 创建服务器
func createServer(client *HentaiAtHomeClient) *server.Server {
	srv, _ := server.NewServer(client)
	return srv
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
