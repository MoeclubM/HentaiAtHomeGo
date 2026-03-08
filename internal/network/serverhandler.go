// Package network 提供服务器通信功能
package network

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/stats"
	"github.com/qwq/hentaiathomego/internal/util"
)

// 动作常量
const (
	ACT_SERVER_STAT           = "server_stat"
	ACT_GET_BLACKLIST         = "get_blacklist"
	ACT_GET_CERTIFICATE       = "get_cert"
	ACT_CLIENT_LOGIN          = "client_login"
	ACT_CLIENT_SETTINGS       = "client_settings"
	ACT_CLIENT_START          = "client_start"
	ACT_CLIENT_SUSPEND        = "client_suspend"
	ACT_CLIENT_RESUME         = "client_resume"
	ACT_CLIENT_STOP           = "client_stop"
	ACT_STILL_ALIVE           = "still_alive"
	ACT_STATIC_RANGE_FETCH    = "srfetch"
	ACT_DOWNLOADER_FETCH      = "dlfetch"
	ACT_DOWNLOADER_FAILREPORT = "dlfails"
	ACT_OVERLOAD              = "overload"
)

// 响应状态常量
const (
	RESPONSE_STATUS_NULL = 0
	RESPONSE_STATUS_OK   = 1
	RESPONSE_STATUS_FAIL = -1
)

// ServerResponse 服务器响应
type ServerResponse struct {
	responseStatus int
	responseText   []string
	failCode       string
	failHost       string
}

// GetResponseStatus 获取响应状态
func (sr *ServerResponse) GetResponseStatus() int {
	return sr.responseStatus
}

// GetResponseText 获取响应文本
func (sr *ServerResponse) GetResponseText() []string {
	return sr.responseText
}

// GetFailCode 获取失败代码
func (sr *ServerResponse) GetFailCode() string {
	return sr.failCode
}

// GetFailHost 获取失败主机
func (sr *ServerResponse) GetFailHost() string {
	return sr.failHost
}

// ServerHandler 服务器处理器
type ServerHandler struct {
	client                   Client
	loginValidated           bool
	lastOverloadNotification int64
}

// Client 服务器客户端接口
type Client interface {
	GetInputQueryHandler() util.InputQueryHandler
	DieWithError(error string)
	PromptForIDAndKey()
}

// NewServerHandler 创建新的服务器处理器
func NewServerHandler(client Client) *ServerHandler {
	return &ServerHandler{
		client:                   client,
		loginValidated:           false,
		lastOverloadNotification: 0,
	}
}

// GetServerConnectionURL 获取服务器连接 URL
func GetServerConnectionURL(act string) string {
	return GetServerConnectionURLWithAdd(act, "")
}

// GetServerConnectionURLWithAdd 获取带附加参数的服务器连接 URL
func GetServerConnectionURLWithAdd(act, add string) string {
	settings := config.GetSettings()

	var serverURL string
	if act == ACT_SERVER_STAT {
		serverURL = fmt.Sprintf("%s/%sclientbuild=%d&act=%s",
			config.CLIENT_RPC_PROTOCOL+settings.GetRPCServerHost(),
			settings.GetRPCPath(),
			config.CLIENT_BUILD,
			act)
	} else {
		serverURL = fmt.Sprintf("%s/%s%s",
			config.CLIENT_RPC_PROTOCOL+settings.GetRPCServerHost(),
			settings.GetRPCPath(),
			getURLQueryString(act, add))
	}

	return serverURL
}

// getURLQueryString 获取 URL 查询字符串
func getURLQueryString(act, add string) string {
	settings := config.GetSettings()

	correctedTime := settings.GetServerTime()
	actkey := getSHA1String(fmt.Sprintf("hentai@home-%s-%s-%d-%d-%s",
		act, add, settings.GetClientID(), correctedTime, settings.GetClientKey()))

	return fmt.Sprintf("clientbuild=%d&act=%s&add=%s&cid=%d&acttime=%d&actkey=%s",
		config.CLIENT_BUILD, act, add, settings.GetClientID(), correctedTime, actkey)
}

// getSHA1String 计算 SHA-1 字符串
func getSHA1String(s string) string {
	hash := sha1.Sum([]byte(s))
	return strings.ToLower(hex.EncodeToString(hash[:]))
}

// GetServerResponse 获取服务器响应
func GetServerResponse(act string, handler *ServerHandler) *ServerResponse {
	serverURL := GetServerConnectionURL(act)
	return getServerResponseWithURL(serverURL, handler, act)
}

// GetServerResponseWithURL 获取服务器响应（带 URL）
func GetServerResponseWithURL(serverURL string, handler *ServerHandler) *ServerResponse {
	return getServerResponseWithURL(serverURL, handler, "")
}

func failHostFromURL(serverURL string) string {
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return ""
	}

	host := parsedURL.Hostname()
	if host == "" {
		host = parsedURL.Host
	}

	return strings.ToLower(host)
}

func newNullServerResponse(serverURL, failCode string) *ServerResponse {
	return &ServerResponse{
		responseStatus: RESPONSE_STATUS_NULL,
		failCode:       failCode,
		failHost:       failHostFromURL(serverURL),
	}
}

func splitResponseLines(serverResponse string) []string {
	split := strings.Split(serverResponse, "\n")
	for len(split) > 1 && split[len(split)-1] == "" {
		split = split[:len(split)-1]
	}
	return split
}

func isValidAbsoluteURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return parsedURL.Scheme != "" && parsedURL.Host != ""
}

// getServerResponseWithURL 从 URL 获取服务器响应
func getServerResponseWithURL(serverURL string, handler *ServerHandler, retryAct string) *ServerResponse {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 60 * time.Minute,
	}
	client := &http.Client{
		Transport: transport,
	}

	req, err := http.NewRequest(http.MethodGet, serverURL, nil)
	if err != nil {
		return newNullServerResponse(serverURL, "NO_RESPONSE")
	}
	req.Header.Set("Connection", "Close")
	req.Header.Set("User-Agent", "Hentai@Home "+config.CLIENT_VERSION)

	resp, err := client.Do(req)
	if err != nil {
		return newNullServerResponse(serverURL, "NO_RESPONSE")
	}

	if resp.StatusCode != http.StatusOK || resp.ContentLength < 0 || resp.ContentLength > 10485760 {
		resp.Body.Close()
		return newNullServerResponse(serverURL, "NO_RESPONSE")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return newNullServerResponse(serverURL, "NO_RESPONSE")
	}

	serverResponse := string(body)
	util.Debug("收到响应: %s", serverResponse)

	split := splitResponseLines(serverResponse)
	if len(split) < 1 {
		return newNullServerResponse(serverURL, "NO_RESPONSE")
	}

	if strings.HasPrefix(split[0], "TEMPORARILY_UNAVAILABLE") {
		return newNullServerResponse(serverURL, "TEMPORARILY_UNAVAILABLE")
	}

	if split[0] == "OK" {
		return &ServerResponse{
			responseStatus: RESPONSE_STATUS_OK,
			responseText:   split[1:],
		}
	}

	if split[0] == "KEY_EXPIRED" && handler != nil && retryAct != "" {
		util.Warning("服务器报告密钥已过期；尝试从服务器刷新时间并重试")
		handler.RefreshServerStat()
		return getServerResponseWithURL(GetServerConnectionURL(retryAct), nil, "")
	}

	return &ServerResponse{
		responseStatus: RESPONSE_STATUS_FAIL,
		failCode:       split[0],
		failHost:       failHostFromURL(serverURL),
	}
}

// simpleNotification 简单通知
func (sh *ServerHandler) simpleNotification(act, humanReadable string) bool {
	sr := GetServerResponse(act, sh)

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		settings := config.GetSettings()
		settings.MarkRPCServerFailure(sr.GetFailHost())
	}

	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		util.Debug("%s 通知成功", humanReadable)
		return true
	}

	util.Warning("%s 通知失败", humanReadable)
	return false
}

// NotifySuspend 通知暂停
func (sh *ServerHandler) NotifySuspend() bool {
	return sh.simpleNotification(ACT_CLIENT_SUSPEND, "暂停")
}

// NotifyResume 通知恢复
func (sh *ServerHandler) NotifyResume() bool {
	return sh.simpleNotification(ACT_CLIENT_RESUME, "恢复")
}

// NotifyShutdown 通知关闭
func (sh *ServerHandler) NotifyShutdown() bool {
	return sh.simpleNotification(ACT_CLIENT_STOP, "关闭")
}

// NotifyOverload 通知过载
func (sh *ServerHandler) NotifyOverload() bool {
	now := time.Now().UnixMilli()

	if sh.lastOverloadNotification < now-30000 {
		sh.lastOverloadNotification = now
		return sh.simpleNotification(ACT_OVERLOAD, "过载")
	}

	return false
}

// NotifyStart 通知启动
func (sh *ServerHandler) NotifyStart() bool {
	sr := GetServerResponse(ACT_CLIENT_START, sh)

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		settings := config.GetSettings()
		settings.MarkRPCServerFailure(sr.GetFailHost())
	}

	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		util.Info("启动通知成功。服务器可能需要短暂时间来注册此客户端。")
		return true
	}

	failCode := sr.GetFailCode()
	util.Warning("启动失败: %s", failCode)
	util.Debug("完整响应: %v", sr)

	if strings.HasPrefix(failCode, "FAIL_CONNECT_TEST") {
		util.Info("")
		util.Info("************************************************************************************************************************************")
		util.Info("客户端未能通过外部连接测试。")
		util.Info("服务器无法连接到客户端，这通常意味着它无法从互联网访问。")
		settings := config.GetSettings()
		util.Info("如果您在 NAT 和/或防火墙后面，请检查端口 %d 已打开并转发到此计算机。", settings.GetClientPort())
		util.Info("************************************************************************************************************************************")
		util.Info("")
		return false
	}

	if strings.HasPrefix(failCode, "FAIL_OTHER_CLIENT_CONNECTED") {
		util.Info("服务器检测到此计算机或本地网络上已连接另一个客户端。")
		util.Info("每个公共 IPv4 地址只能运行一个客户端。")
		sh.client.DieWithError("FAIL_OTHER_CLIENT_CONNECTED")
		return false
	}

	if strings.HasPrefix(failCode, "FAIL_CID_IN_USE") {
		util.Info("服务器检测到另一个客户端正在使用此客户端 ID。")
		sh.client.DieWithError("FAIL_CID_IN_USE")
		return false
	}

	return false
}

// GetBlacklist 获取黑名单
func (sh *ServerHandler) GetBlacklist(deltaTime int64) []string {
	blacklistURL := GetServerConnectionURLWithAdd(ACT_GET_BLACKLIST, strconv.FormatInt(deltaTime, 10))
	sr := GetServerResponseWithURL(blacklistURL, sh)

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		settings := config.GetSettings()
		settings.MarkRPCServerFailure(sr.GetFailHost())
	}

	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		return sr.GetResponseText()
	}

	return nil
}

// StillAliveTest 存活测试
func (sh *ServerHandler) StillAliveTest(resume bool) {
	cs := NewCakeSphere(sh, sh.client)
	cs.StillAlive(resume)
}

// GetSettings 获取设置
func (sh *ServerHandler) GetSettings() *config.Settings {
	return config.GetSettings()
}

// LoadClientSettingsFromServer 从服务器加载客户端设置
func (sh *ServerHandler) LoadClientSettingsFromServer() {
	stats.SetProgramStatus("正在从服务器加载设置...")
	util.Info("连接到 Hentai@Home 服务器以注册客户端 ID %d...", config.GetSettings().GetClientID())

	for {
		if !sh.RefreshServerStat() {
			sh.client.DieWithError("无法从服务器获取初始状态。")
			return
		}

		util.Info("从服务器读取 Hentai@Home 客户端设置...")
		sr := GetServerResponse(ACT_CLIENT_LOGIN, sh)

		if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
			sh.loginValidated = true
			util.Info("应用设置...")
			settings := config.GetSettings()
			settings.ParseAndUpdateSettings(sr.GetResponseText())
			util.Info("完成应用设置")
			break
		}

		if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
			sh.client.DieWithError("无法从服务器获取登录响应。")
			return
		}

		util.Warning("身份验证失败，请重新输入您的客户端 ID 和密钥（代码: %s）", sr.GetFailCode())
		settings := config.GetSettings()
		_ = settings
		sh.client.PromptForIDAndKey()
	}
}

// RefreshServerSettings 刷新服务器设置
func (sh *ServerHandler) RefreshServerSettings() bool {
	util.Info("从服务器刷新 Hentai@Home 客户端设置...")
	sr := GetServerResponse(ACT_CLIENT_SETTINGS, sh)

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		settings := config.GetSettings()
		settings.MarkRPCServerFailure(sr.GetFailHost())
	}

	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		settings := config.GetSettings()
		settings.ParseAndUpdateSettings(sr.GetResponseText())
		util.Info("完成应用设置")
		return true
	}

	util.Warning("刷新设置失败")
	return false
}

// RefreshServerStat 刷新服务器状态
func (sh *ServerHandler) RefreshServerStat() bool {
	stats.SetProgramStatus("正在从服务器获取初始状态...")
	sr := GetServerResponse(ACT_SERVER_STAT, sh)

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		settings := config.GetSettings()
		settings.MarkRPCServerFailure(sr.GetFailHost())
	}

	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		settings := config.GetSettings()
		settings.ParseAndUpdateSettings(sr.GetResponseText())
		return true
	}

	return false
}

// GetStaticRangeFetchURL 获取静态范围获取 URL
func (sh *ServerHandler) GetStaticRangeFetchURL(fileIndex, xRes, fileID string) []string {
	add := fmt.Sprintf("%s;%s;%s", fileIndex, xRes, fileID)
	requestURL := GetServerConnectionURLWithAdd(ACT_STATIC_RANGE_FETCH, add)
	sr := GetServerResponseWithURL(requestURL, sh)

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		settings := config.GetSettings()
		settings.MarkRPCServerFailure(sr.GetFailHost())
	}

	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		response := sr.GetResponseText()
		urls := make([]string, 0, len(response))

		for _, s := range response {
			if s != "" {
				if !isValidAbsoluteURL(s) {
					util.Warning("静态范围获取返回了无效 URL: %s", s)
					return nil
				}
				urls = append(urls, s)
			}
		}

		if len(urls) == 0 {
			return nil
		}

		return urls
	}

	util.Info("无法请求 %s 的静态范围下载链接。", fileID)
	return nil
}

// GetDownloaderFetchURL 获取下载器获取 URL
func (sh *ServerHandler) GetDownloaderFetchURL(gid, page, fileIndex int, xRes string, fileRetry int) string {
	add := fmt.Sprintf("%d;%d;%d;%s;%d", gid, page, fileIndex, xRes, fileRetry)
	requestURL := GetServerConnectionURLWithAdd(ACT_DOWNLOADER_FETCH, add)
	sr := GetServerResponseWithURL(requestURL, sh)

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		settings := config.GetSettings()
		settings.MarkRPCServerFailure(sr.GetFailHost())
	}

	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		response := sr.GetResponseText()
		if len(response) > 0 && isValidAbsoluteURL(response[0]) {
			return response[0]
		}
	}

	util.Info("无法请求 fileindex=%d 的画廊文件 URL。", fileIndex)
	return ""
}

// ReportDownloaderFailures 报告下载器失败
func (sh *ServerHandler) ReportDownloaderFailures(failures []string) {
	if failures == nil || len(failures) < 1 || len(failures) > 50 {
		return
	}

	var s strings.Builder
	for i, failure := range failures {
		s.WriteString(failure)
		if i < len(failures)-1 {
			s.WriteString(";")
		}
	}

	requestURL := GetServerConnectionURLWithAdd(ACT_DOWNLOADER_FAILREPORT, s.String())
	sr := GetServerResponseWithURL(requestURL, sh)

	if sr.GetResponseStatus() == RESPONSE_STATUS_NULL {
		settings := config.GetSettings()
		settings.MarkRPCServerFailure(sr.GetFailHost())
	}

	status := "FAIL"
	if sr.GetResponseStatus() == RESPONSE_STATUS_OK {
		status = "OK"
	}

	util.Debug("报告了 %d 个下载失败，响应: %s", len(failures), status)
}

// IsLoginValidated 检查登录是否已验证
func (sh *ServerHandler) IsLoginValidated() bool {
	return sh.loginValidated
}
