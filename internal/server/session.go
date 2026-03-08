// Package server 提供会话处理功能
package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/download"
	"github.com/qwq/hentaiathomego/internal/server/processors"
	"github.com/qwq/hentaiathomego/internal/stats"
	"github.com/qwq/hentaiathomego/internal/util"
	"github.com/qwq/hentaiathomego/pkg/hvfile"
)

const CRLF = "\r\n"

// Session HTTP 会话
type Session struct {
	conn               *tls.Conn
	connID             int
	localNetworkAccess bool
	server             *Server
	sessionStartTime   time.Time
	lastPacketSend     time.Time
	closed             uint32
	response           *Response
}

// NewSession 创建新的会话
func NewSession(conn *tls.Conn, connID int, localNetworkAccess bool, server *Server) *Session {
	return &Session{
		conn:               conn,
		connID:             connID,
		localNetworkAccess: localNetworkAccess,
		server:             server,
		sessionStartTime:   time.Now(),
		lastPacketSend:     time.Now(),
	}
}

// HandleSession 处理会话
func (s *Session) HandleSession() {
	go s.run()
}

// run 会话主循环
func (s *Session) run() {
	defer func() {
		s.connectionFinished()
	}()

	s.conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	reader := bufio.NewReader(s.conn)
	writer := bufio.NewWriter(s.conn)

	// 扫描 HTTP 请求头
	var request string
	rcvdBytes := 0
	readLines := 0

	getheadPattern := regexp.MustCompile(`(?i)^((GET)|(HEAD)).*`)

	for {
		line, err := s.readLine(reader)
		if err != nil {
			break
		}

		rcvdBytes += len(line)

		if getheadPattern.MatchString(line) {
			request = line
		} else if line == "" {
			// 空行表示请求头结束
			break
		}

		readLines++
		if readLines >= 100 || rcvdBytes >= 10000 {
			break
		}
	}

	// 解析请求并获取响应处理器
	s.response = NewResponse(s)
	s.response.ParseRequest(request, s.localNetworkAccess)
	if s.response.ShouldAbortConnection() {
		return
	}
	responseProcessor := s.response.GetResponseProcessor()
	if responseProcessor == nil {
		return
	}
	statusCode := s.response.GetResponseStatusCode()
	contentLength := responseProcessor.GetContentLength()

	// 构建响应头
	header := s.getHTTPStatusHeader(statusCode)
	header += responseProcessor.GetHeader()
	header += fmt.Sprintf("Date: %s GMT%s", time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05"), CRLF)
	header += fmt.Sprintf("Server: Genetic Lifeform and Distributed Open Server %s%s", config.CLIENT_VERSION, CRLF)
	header += fmt.Sprintf("Connection: close%s", CRLF)
	header += fmt.Sprintf("Content-Type: %s%s", responseProcessor.GetContentType(), CRLF)

	if contentLength > 0 {
		header += fmt.Sprintf("Cache-Control: public, max-age=31536000%s", CRLF)
		header += fmt.Sprintf("Content-Length: %d%s", contentLength, CRLF)
	}

	header += CRLF

	// 写入响应头
	headerBytes := []byte(header)
	if request != "" && contentLength > 0 {
		bufferSize := minInt(contentLength+len(headerBytes)+32, 524288)
		settings := config.GetSettings()
		if !settings.IsUseLessMemory() {
			bufferSize = minInt(bufferSize, int(float64(settings.GetThrottleBytesPerSec())*0.2))
		}
		// TODO: 设置发送缓冲区大小
	}

	bwm := s.server.GetBandwidthMonitor()

	if bwm != nil && !s.localNetworkAccess {
		bwm.WaitForQuota(nil, len(headerBytes))
	}

	writer.Write(headerBytes)
	writer.Flush()

	if !s.localNetworkAccess {
		stats.BytesSent(len(headerBytes))
	}

	if s.response.IsRequestHeadOnly() {
		// HEAD 请求，已完成
		info := fmt.Sprintf("{%d %-17s} Code=%d ", s.connID, s.getRemoteAddr(), statusCode)
		util.Info("%s%s", info, request)
		return
	}

	// GET 请求，处理内容
	info := fmt.Sprintf("{%d %-17s} Code=%d Bytes=%-8d ", s.connID, s.getRemoteAddr(), statusCode, contentLength)

	if request != "" {
		util.Info("%s%s", info, request)
	}

	startTime := time.Now()

	if contentLength > 0 {
		writtenBytes := 0

		for writtenBytes < contentLength {
			s.lastPacketSend = time.Now()

			tcpBuffer, err := responseProcessor.(interface{ GetPreparedTCPBuffer() ([]byte, error) }).GetPreparedTCPBuffer()
			if err != nil {
				util.Debug("获取 TCP 缓冲区错误: %v", err)
				break
			}

			if bwm != nil && !s.localNetworkAccess {
				bwm.WaitForQuota(nil, len(tcpBuffer))
			}

			writer.Write(tcpBuffer)
			writtenBytes += len(tcpBuffer)

			if !s.localNetworkAccess {
				stats.BytesSent(len(tcpBuffer))
			}
		}
	}

	writer.Flush()

	sendTime := time.Since(startTime).Milliseconds()
	util.Info("%s完成处理请求，耗时 %.2f 秒", info, float64(sendTime)/1000.0)
}

// connectionFinished 连接完成
func (s *Session) connectionFinished() {
	if s.response != nil {
		s.response.RequestCompleted()
	}

	s.server.RemoveSession(s)
}

// DoTimeoutCheck 执行超时检查
func (s *Session) DoTimeoutCheck(now time.Time) bool {
	if s.lastPacketSend.Before(now.Add(-1*time.Second)) && s.isClosed() {
		return true
	}

	startTimeout := 30 * time.Second
	if s.response != nil {
		startTimeout = 180 * time.Second
		if s.response.IsServercmd() {
			startTimeout = 1800 * time.Second
		}
	}

	if !s.sessionStartTime.IsZero() && s.sessionStartTime.Before(now.Add(-startTimeout)) {
		return true
	}

	if !s.lastPacketSend.IsZero() && s.lastPacketSend.Before(now.Add(-30*time.Second)) {
		return true
	}

	return false
}

// ForceClose 强制关闭连接
func (s *Session) ForceClose() {
	if atomic.CompareAndSwapUint32(&s.closed, 0, 1) {
		util.Debug("关闭会话 %d 的套接字", s.connID)
		s.conn.Close()
		util.Debug("已关闭会话 %d 的套接字", s.connID)
	}
}

// GetHTTPServer 获取 HTTP 服务器
func (s *Session) GetHTTPServer() *Server {
	return s.server
}

// GetSocketInetAddress 获取套接字地址
func (s *Session) GetSocketInetAddress() net.Addr {
	return s.conn.RemoteAddr()
}

// IsLocalNetworkAccess 检查是否是本地网络访问
func (s *Session) IsLocalNetworkAccess() bool {
	return s.localNetworkAccess
}

// String 返回会话字符串表示
func (s *Session) String() string {
	return fmt.Sprintf("{%d %-17s}", s.connID, s.getRemoteAddr())
}

// readLine 读取一行
func (s *Session) readLine(reader *bufio.Reader) (string, error) {
	var line []byte
	maxLen := 1000

	for {
		b, err := reader.ReadByte()
		if err != nil {
			if len(line) == 0 {
				return "", err
			}
			break
		}

		if b == '\r' {
			// 检查下一个字符是否是 \n
			nextByte, err := reader.ReadByte()
			if err != nil {
				break
			}
			if nextByte != '\n' {
				// 不是 \n，放回缓冲区
				reader.UnreadByte()
			}
			break
		}

		if b == '\n' {
			break
		}

		if len(line) < maxLen {
			line = append(line, b)
		} else {
			// 超过最大长度，继续读取直到行尾
			continue
		}
	}

	return string(line), nil
}

// getRemoteAddr 获取远程地址
func (s *Session) getRemoteAddr() string {
	addr := s.conn.RemoteAddr()
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		return tcpAddr.IP.String()
	}
	return addr.String()
}

// isClosed 检查连接是否已关闭
func (s *Session) isClosed() bool {
	return atomic.LoadUint32(&s.closed) != 0
}

// getHTTPStatusHeader 获取 HTTP 状态头
func (s *Session) getHTTPStatusHeader(statusCode int) string {
	switch statusCode {
	case 200:
		return "HTTP/1.1 200 OK" + CRLF
	case 301:
		return "HTTP/1.1 301 Moved Permanently" + CRLF
	case 400:
		return "HTTP/1.1 400 Bad Request" + CRLF
	case 403:
		return "HTTP/1.1 403 Permission Denied" + CRLF
	case 404:
		return "HTTP/1.1 404 Not Found" + CRLF
	case 405:
		return "HTTP/1.1 405 Method Not Allowed" + CRLF
	case 418:
		return "HTTP/1.1 418 I'm a teapot" + CRLF
	case 501:
		return "HTTP/1.1 501 Not Implemented" + CRLF
	case 502:
		return "HTTP/1.1 502 Bad Gateway" + CRLF
	default:
		return "HTTP/1.1 500 Internal Server Error" + CRLF
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Response HTTP 响应
type Response struct {
	session            *Session
	requestHeadOnly    bool
	servercmd          bool
	abortConnection    bool
	responseStatusCode int
	processor          processors.HTTPResponseProcessorInterface
}

// NewResponse 创建新的响应
func NewResponse(session *Session) *Response {
	return &Response{
		session:            session,
		servercmd:          false,
		requestHeadOnly:    false,
		responseStatusCode: 500,
	}
}

// ParseRequest 解析请求
func (r *Response) ParseRequest(request string, localNetworkAccess bool) {
	if request == "" {
		util.Debug("%v 客户端未发送请求", r.session)
		r.responseStatusCode = 400
		return
	}

	requestParts := strings.SplitN(strings.TrimSpace(request), " ", 3)

	if len(requestParts) != 3 {
		util.Debug("%v 无效的 HTTP 请求格式", r.session)
		r.responseStatusCode = 400
		return
	}

	if !(strings.EqualFold(requestParts[0], "GET") || strings.EqualFold(requestParts[0], "HEAD")) || !strings.HasPrefix(requestParts[2], "HTTP/") {
		util.Debug("%v HTTP 请求不是 GET/HEAD 或 HTTP/ 版本无效", r.session)
		r.responseStatusCode = 405
		return
	}

	r.requestHeadOnly = strings.EqualFold(requestParts[0], "HEAD")

	// 解析 URL
	uri := strings.ReplaceAll(requestParts[1], "%3d", "=")
	// 兼容 RFC2616: 允许 absolute URI
	if strings.HasPrefix(strings.ToLower(uri), "http://") {
		if idx := strings.Index(uri[7:], "/"); idx >= 0 {
			uri = uri[7+idx:]
		} else {
			uri = "/"
		}
	}
	urlParts := strings.Split(uri, "/")

	if len(urlParts) < 2 || urlParts[0] != "" {
		util.Debug("%v 请求的 URL 无效或不支持", r.session)
		r.responseStatusCode = 404
		return
	}

	if urlParts[1] == "h" {
		r.processFileRequest(urlParts, localNetworkAccess)
	} else if urlParts[1] == "servercmd" {
		r.processServerCommand(urlParts)
	} else if urlParts[1] == "t" {
		r.processSpeedTest(urlParts)
	} else if len(urlParts) == 2 {
		if urlParts[1] == "favicon.ico" {
			r.processor = processors.NewHTTPResponseProcessorText("")
			r.processor.AddHeaderField("Location", "https://e-hentai.org/favicon.ico")
			r.responseStatusCode = 301
		} else if urlParts[1] == "robots.txt" {
			r.processor = processors.NewHTTPResponseProcessorTextWithEncoding("User-agent: *\nDisallow: /", "text/plain", "ISO-8859-1")
			r.responseStatusCode = 200
		} else {
			util.Debug("%v 无效的请求类型 '%s'", r.session, urlParts[1])
			r.responseStatusCode = 404
		}
	} else {
		util.Debug("%v 无效的请求类型 '%s'", r.session, urlParts[1])
		r.responseStatusCode = 404
	}
}

// processFileRequest 处理文件请求
func (r *Response) processFileRequest(urlParts []string, localNetworkAccess bool) {
	if len(urlParts) < 4 {
		r.responseStatusCode = 400
		return
	}

	fileID := urlParts[2]
	requestedHVFile, hvErr := hvfile.GetHVFileFromFileid(fileID)

	additional := util.ParseAdditional(urlParts[3])
	keystampRejected := true

	// 验证 keystamp
	keystampParts := strings.Split(additional["keystamp"], "-")
	if len(keystampParts) == 2 {
		keystampTime, err := strconv.Atoi(keystampParts[0])
		if err == nil {
			settings := config.GetSettings()
			if abs(settings.GetServerTime()-keystampTime) < 900 {
				expectedKeystamp := util.GetSHA1String(fmt.Sprintf("%d-%s-%s-hotlinkthis", keystampTime, fileID, settings.GetClientKey()))[:10]
				if strings.EqualFold(keystampParts[1], expectedKeystamp) {
					keystampRejected = false
				}
			}
		}
	}

	if keystampRejected {
		r.responseStatusCode = 403
		return
	}

	fileindex := additional["fileindex"]
	xres := additional["xres"]

	if requestedHVFile == nil || hvErr != nil || fileindex == "" || xres == "" || !regexp.MustCompile(`^\d+$`).MatchString(fileindex) || !regexp.MustCompile(`^(org|\d+)$`).MatchString(xres) {
		util.Debug("%v 无效或缺失参数", r.session)
		r.responseStatusCode = 404
		return
	}

	// 检查文件是否存在于缓存中
	requestedFile := requestedHVFile.GetLocalFileRef()
	if util.FileExists(requestedFile) {
		if size, err := util.GetFileSize(requestedFile); err == nil && size == int64(requestedHVFile.GetSize()) {
			cacheHandler := r.session.server.client.GetCacheHandler()
			verifyFileIntegrity := false
			if cacheHandler != nil && cacheHandler.MarkRecentlyAccessed(requestedHVFile) {
				verifyFileIntegrity = !config.GetSettings().IsDisableFileVerification() && !cacheHandler.IsFileVerificationOnCooldown()
			}

			fileProcessor := processors.NewHTTPResponseProcessorFile(requestedHVFile, verifyFileIntegrity)
			fileProcessor.SetCacheHandler(cacheHandler)
			r.processor = fileProcessor
			r.responseStatusCode = 200
			return
		}
	}

	{
		// 文件不存在，需要代理请求
		sources := r.session.server.client.GetServerHandler().GetStaticRangeFetchURL(fileindex, xres, fileID)
		if sources == nil || len(sources) == 0 {
			util.Debug("%v fileindex=%s xres=%s fileid=%s 的回源地址为空", r.session, fileindex, xres, fileID)
			r.responseStatusCode = 404
			return
		}

		proxyProcessor := processors.NewHTTPResponseProcessorProxy(fileID)
		proxyDownloader := download.NewProxyFileDownloader(r.session.server.client.GetCacheHandler(), fileID, sources)
		proxyProcessor.SetProxyDownloader(proxyDownloader)
		r.processor = proxyProcessor
		r.responseStatusCode = 200
	}
}

// processServerCommand 处理服务器命令
func (r *Response) processServerCommand(urlParts []string) {
	settings := config.GetSettings()

	if !settings.IsValidRPCServer(r.session.GetSocketInetAddress().(*net.TCPAddr).IP) {
		util.Debug("%v 从未授权 IP 地址获取 servercmd", r.session)
		r.responseStatusCode = 403
		return
	}

	if len(urlParts) < 6 {
		util.Debug("%v 格式错误的 servercmd", r.session)
		r.responseStatusCode = 403
		return
	}

	command := urlParts[2]
	additional := urlParts[3]
	commandTime, err := strconv.Atoi(urlParts[4])
	if err != nil {
		r.abortConnection = true
		return
	}
	key := urlParts[5]

	if abs(commandTime-settings.GetServerTime()) > config.MAX_KEY_TIME_DRIFT {
		util.Debug("%v servercmd 密钥过期", r.session)
		r.responseStatusCode = 403
		return
	}

	expectedKey := util.GetSHA1String(fmt.Sprintf("hentai@home-servercmd-%s-%s-%d-%d-%s",
		command, additional, settings.GetClientID(), commandTime, settings.GetClientKey()))
	if key != expectedKey {
		util.Debug("%v servercmd 密钥不正确", r.session)
		r.responseStatusCode = 403
		return
	}

	r.responseStatusCode = 200
	r.servercmd = true

	addTable := util.ParseAdditional(additional)

	commandLower := strings.ToLower(command)

	// 处理命令
	switch commandLower {
	case "still_alive":
		r.processor = processors.NewHTTPResponseProcessorText("I feel FANTASTIC and I'm still alive")
	case "threaded_proxy_test":
		r.processor = r.processThreadedProxyTest(addTable)
	case "speed_test":
		testSize := 1000000
		if value, ok := addTable["testsize"]; ok {
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil {
				r.processor = processors.NewHTTPResponseProcessorText("INVALID_COMMAND")
				return
			}
			testSize = parsed
		}
		r.processor = processors.NewHTTPResponseProcessorSpeedtest(testSize)
	case "refresh_settings":
		r.session.server.client.GetServerHandler().RefreshServerSettings()
		r.processor = processors.NewHTTPResponseProcessorText("")
	case "start_downloader":
		r.session.server.client.StartDownloader()
		r.processor = processors.NewHTTPResponseProcessorText("")
	case "refresh_certs":
		r.session.server.client.SetCertRefresh()
		r.processor = processors.NewHTTPResponseProcessorText("")
	default:
		r.processor = processors.NewHTTPResponseProcessorText("INVALID_COMMAND")
	}
}

func (r *Response) processThreadedProxyTest(addTable map[string]string) processors.HTTPResponseProcessorInterface {
	hostname := addTable["hostname"]
	if hostname == "" {
		return processors.NewHTTPResponseProcessorText("INVALID_COMMAND")
	}

	protocol := addTable["protocol"]
	if protocol == "" {
		protocol = "http"
	}

	port, err := strconv.Atoi(addTable["port"])
	if err != nil || port < -1 {
		return processors.NewHTTPResponseProcessorText("INVALID_COMMAND")
	}

	testSize, err := strconv.Atoi(addTable["testsize"])
	if err != nil {
		return processors.NewHTTPResponseProcessorText("INVALID_COMMAND")
	}

	testCount, err := strconv.Atoi(addTable["testcount"])
	if err != nil {
		return processors.NewHTTPResponseProcessorText("INVALID_COMMAND")
	}

	testTime, err := strconv.Atoi(addTable["testtime"])
	if err != nil {
		return processors.NewHTTPResponseProcessorText("INVALID_COMMAND")
	}

	testKey := addTable["testkey"]
	if testKey == "" {
		return processors.NewHTTPResponseProcessorText("INVALID_COMMAND")
	}

	hostPort := hostname
	if port >= 0 {
		hostPort = fmt.Sprintf("%s:%d", hostname, port)
	}
	if _, err := url.Parse(fmt.Sprintf("%s://%s/t/%d/%d/%s/0", protocol, hostPort, testSize, testTime, testKey)); err != nil {
		r.abortConnection = true
		return nil
	}

	util.Debug("运行 threaded_proxy_test: host=%s protocol=%s port=%d testsize=%d testcount=%d testtime=%d",
		hostname, protocol, port, testSize, testCount, testTime)

	type testResult struct {
		success bool
		timeMS  int64
	}

	channelSize := testCount
	if channelSize < 0 {
		channelSize = 0
	}
	results := make(chan testResult, channelSize)
	var waitGroup sync.WaitGroup

	for i := 0; i < testCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()

			const javaMaxRandom = (1 << 31) - 1
			hostPort := hostname
			if port >= 0 {
				hostPort = fmt.Sprintf("%s:%d", hostname, port)
			}
			source := fmt.Sprintf("%s://%s/t/%d/%d/%s/%d", protocol, hostPort, testSize, testTime, testKey, util.RandomInt(javaMaxRandom))
			downloader := download.NewFileDownloader(source, 10000, 60000)

			err := downloader.DownloadFile()

			if err == nil && downloader.GetContentLength() >= testSize {
				results <- testResult{success: true, timeMS: downloader.GetDownloadTimeMillis()}
				return
			}
			results <- testResult{success: false, timeMS: 0}
		}()
	}

	waitGroup.Wait()
	close(results)

	successfulTests := 0
	var totalTimeMillis int64
	for result := range results {
		if result.success {
			successfulTests++
			totalTimeMillis += result.timeMS
		}
	}

	util.Debug("threaded_proxy_test 完成: successfulTests=%d totalTimeMillis=%d", successfulTests, totalTimeMillis)
	return processors.NewHTTPResponseProcessorText(fmt.Sprintf("OK:%d-%d", successfulTests, totalTimeMillis))
}

// processSpeedTest 处理速度测试
func (r *Response) processSpeedTest(urlParts []string) {
	if len(urlParts) < 5 {
		r.responseStatusCode = 400
		return
	}

	testSize, err := strconv.Atoi(urlParts[2])
	if err != nil {
		r.abortConnection = true
		return
	}

	testTime, err := strconv.Atoi(urlParts[3])
	if err != nil {
		r.abortConnection = true
		return
	}

	testKey := urlParts[4]

	settings := config.GetSettings()
	if abs(testTime-settings.GetServerTime()) > config.MAX_KEY_TIME_DRIFT {
		util.Debug("%v 速度测试请求密钥过期", r.session)
		r.responseStatusCode = 403
		return
	}

	expectedKey := util.GetSHA1String(fmt.Sprintf("hentai@home-speedtest-%d-%d-%d-%s",
		testSize, testTime, settings.GetClientID(), settings.GetClientKey()))
	if testKey != expectedKey {
		util.Debug("%v 速度测试请求密钥无效", r.session)
		r.responseStatusCode = 403
		return
	}

	util.Debug("发送速度测试，testsize=%d testtime=%d testkey=%s", testSize, testTime, testKey)

	r.responseStatusCode = 200
	r.processor = processors.NewHTTPResponseProcessorSpeedtest(testSize)
}

// GetResponseProcessor 获取响应处理器
func (r *Response) GetResponseProcessor() processors.HTTPResponseProcessorInterface {
	if r.abortConnection {
		return nil
	}

	if r.processor == nil {
		r.processor = processors.NewHTTPResponseProcessorText(fmt.Sprintf("An error has occurred. (%d)", r.responseStatusCode))
		if r.responseStatusCode == 405 {
			r.processor.AddHeaderField("Allow", "GET,HEAD")
		}
	}

	// 初始化处理器
	if fileProcessor, ok := r.processor.(*processors.HTTPResponseProcessorFile); ok {
		r.responseStatusCode = fileProcessor.Initialize()
	} else if proxyProcessor, ok := r.processor.(*processors.HTTPResponseProcessorProxy); ok {
		r.responseStatusCode = proxyProcessor.Initialize()
	} else if _, ok := r.processor.(*processors.HTTPResponseProcessorSpeedtest); ok {
		stats.SetProgramStatus("运行速度测试...")
	}

	return r.processor
}

// RequestCompleted 请求完成
func (r *Response) RequestCompleted() {
	if r.processor != nil {
		r.processor.RequestCompleted()
		r.processor.Cleanup()
	}
}

// GetResponseStatusCode 获取响应状态码
func (r *Response) GetResponseStatusCode() int {
	return r.responseStatusCode
}

// IsRequestHeadOnly 检查是否是 HEAD 请求
func (r *Response) IsRequestHeadOnly() bool {
	return r.requestHeadOnly
}

// IsServercmd 检查是否是服务器命令
func (r *Response) IsServercmd() bool {
	return r.servercmd
}

// ShouldAbortConnection reports whether request parsing should terminate the connection without a response.
func (r *Response) ShouldAbortConnection() bool {
	return r.abortConnection
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
