// Package server 提供 HTTP 服务器实现
package server

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/qwq/hentaiathomego/internal/cache"
	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/download"
	"github.com/qwq/hentaiathomego/internal/network"
	"github.com/qwq/hentaiathomego/internal/util"
)

// Server HTTP 服务器
type Server struct {
	client              Client
	listener            net.Listener
	bandwidthMonitor    *BandwidthMonitor
	sessions            map[int]*Session
	sessionsMutex       sync.Mutex
	sessionCount        int
	currentConnID       int
	allowNormalConns    bool
	isRestarting        bool
	isTerminated        bool
	floodControlTable   map[string]*FloodControlEntry
	floodControlMutex   sync.Mutex
	certExpiry          time.Time
	localNetworkPattern *regexp.Regexp
}

// Client 服务器客户端接口
// 这里避免引入跨包接口耦合，仅保留启动/关闭所需能力。
type Client interface {
	NotifyOverload()
	IsShuttingDown() bool
	GetCacheHandler() *cache.CacheHandler
	GetServerHandler() *network.ServerHandler
	StartDownloader()
	SetCertRefresh()
	DieWithError(error string)
}

// NewServer 创建新的 HTTP 服务器
func NewServer(client Client) (*Server, error) {
	// 本地网络模式：localhost, 127.x.y.z, 10.0.0.0-10.255.255.255, 172.16.0.0-172.31.255.255, 192.168.0.0-192.168.255.255
	localPattern := regexp.MustCompile(`^((localhost)|(127\.)|(10\.)|(192\.168\.)|(172\.((1[6-9])|(2[0-9])|(3[0-1]))\.)|(169\.254\.)|(::1)|(0:0:0:0:0:0:0:1)|(fc)|(fd)).*$`)

	server := &Server{
		client:              client,
		sessions:            make(map[int]*Session),
		floodControlTable:   make(map[string]*FloodControlEntry),
		localNetworkPattern: localPattern,
	}

	// 如果未禁用带宽监控，创建带宽监控器
	settings := config.GetSettings()
	if !settings.IsDisableBWM() {
		server.bandwidthMonitor = NewBandwidthMonitor()
	}

	return server, nil
}

// StartConnectionListener 启动连接监听器
func (s *Server) StartConnectionListener(port int) error {
	settings := config.GetSettings()

	util.Info("正在从服务器请求证书...")

	certFile := settings.GetDataDir() + "/hathcert.p12"
	certURL := network.GetServerConnectionURL(network.ACT_GET_CERTIFICATE)

	dl := download.NewFileDownloaderWithOutput(certURL, 10000, 300000, certFile, false)
	if err := dl.DownloadFile(); err != nil {
		return fmt.Errorf("无法下载证书: %w", err)
	}

	pfxBytes, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("无法读取证书文件: %w", err)
	}

	privKey, cert, caCerts, err := pkcs12.DecodeChain(pfxBytes, settings.GetClientKey())
	if err != nil {
		return fmt.Errorf("无法解析 PKCS#12 证书: %w", err)
	}

	s.certExpiry = cert.NotAfter
	if s.IsCertExpired() {
		return fmt.Errorf("证书已过期，或系统时间偏差超过一天")
	}

	chain := make([][]byte, 0, 1+len(caCerts))
	chain = append(chain, cert.Raw)
	for _, c := range caCerts {
		chain = append(chain, c.Raw)
	}

	tlsCert := tls.Certificate{
		Certificate: chain,
		PrivateKey:  privKey,
		Leaf:        cert,
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	}

	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), tlsConfig)
	if err != nil {
		return fmt.Errorf("无法启动监听: %w", err)
	}

	s.listener = listener
	s.allowNormalConns = false

	util.Info("内部 HTTP 服务器启动成功，正在监听端口 %d", port)

	// 启动监听 goroutine
	go s.run()

	return nil
}

// run 服务器主循环
func (s *Server) run() {
	defer func() {
		s.isTerminated = true
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.isRestarting || s.client.IsShuttingDown() {
				s.listener = nil
				util.Info("服务器套接字已关闭，将不再接受新连接")
				return
			}
			s.listener = nil
			util.Error("服务器套接字意外终止: %v", err)
			if s.client != nil {
				s.client.DieWithError(err.Error())
			}
			return
		}

		go s.handleConnection(conn.(*tls.Conn))
	}
}

// handleConnection 处理新连接
func (s *Server) handleConnection(conn *tls.Conn) {
	settings := config.GetSettings()

	forceClose := false
	addr := conn.RemoteAddr().(*net.TCPAddr)
	hostAddress := addr.IP.String()
	localNetworkAccess := s.isLocalNetwork(hostAddress)
	apiServerAccess := settings.IsValidRPCServer(addr.IP)

	// 启动期间不接受普通连接
	if !apiServerAccess && !s.allowNormalConns {
		util.Warning("在启动期间拒绝来自 %s 的连接请求", hostAddress)
		forceClose = true
	} else if !apiServerAccess && !localNetworkAccess {
		// API 服务器和本地网络连接不受最大连接数和洪水控制限制

		maxConnections := settings.GetMaxConnections()

		s.sessionsMutex.Lock()
		sessionCount := len(s.sessions)
		s.sessionsMutex.Unlock()

		if sessionCount > maxConnections {
			util.Warning("超过最大允许连接数 (%d)", maxConnections)
			forceClose = true
		} else {
			if sessionCount > maxConnections*8/10 {
				// 通知服务器接近极限
				s.client.NotifyOverload()
			}

			if !settings.IsDisableFloodControl() {
				s.floodControlMutex.Lock()
				fce := s.floodControlTable[hostAddress]
				if fce == nil {
					fce = &FloodControlEntry{
						addr: addr.IP,
					}
					s.floodControlTable[hostAddress] = fce
				}
				s.floodControlMutex.Unlock()

				if !fce.IsBlocked() {
					if !fce.Hit() {
						util.Warning("对 %s 激活洪水控制（阻止 60 秒）", hostAddress)
						forceClose = true
					}
				} else {
					forceClose = true
				}
			}
		}
	}

	if forceClose {
		conn.Close()
		return
	}

	// 创建会话
	connID := s.getNewConnID()
	session := NewSession(conn, connID, localNetworkAccess, s)

	s.sessionsMutex.Lock()
	s.sessions[connID] = session
	s.sessionCount = len(s.sessions)
	s.sessionsMutex.Unlock()

	// TODO: Stats.setOpenConnections(s.sessionCount)

	session.HandleSession()
}

// isLocalNetwork 检查是否是本地网络
func (s *Server) isLocalNetwork(hostAddress string) bool {
	settings := config.GetSettings()
	clientHost := strings.ToLower(strings.TrimPrefix(settings.GetClientHost(), "::ffff:"))
	hostAddress = strings.ToLower(strings.TrimPrefix(hostAddress, "::ffff:"))

	if clientHost == hostAddress {
		return true
	}

	return s.localNetworkPattern.MatchString(hostAddress)
}

// getNewConnID 获取新连接 ID
func (s *Server) getNewConnID() int {
	s.currentConnID++
	return s.currentConnID
}

// RemoveSession 移除会话
func (s *Server) RemoveSession(session *Session) {
	s.sessionsMutex.Lock()
	delete(s.sessions, session.connID)
	s.sessionCount = len(s.sessions)
	s.sessionsMutex.Unlock()

	// TODO: Stats.setOpenConnections(s.sessionCount)
}

// AllowNormalConnections 允许普通连接
func (s *Server) AllowNormalConnections() {
	s.allowNormalConns = true
}

// StopConnectionListener 停止连接监听器
func (s *Server) StopConnectionListener(restart bool) {
	s.isRestarting = restart

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
}

// IsThreadTerminated 检查线程是否已终止
func (s *Server) IsThreadTerminated() bool {
	return s.isTerminated
}

// IsCertExpired 检查证书是否过期
func (s *Server) IsCertExpired() bool {
	return time.Now().Add(24 * time.Hour).After(s.certExpiry)
}

// PruneFloodControlTable 清理洪水控制表
func (s *Server) PruneFloodControlTable() {
	s.floodControlMutex.Lock()
	defer s.floodControlMutex.Unlock()

	now := time.Now()
	for key, fce := range s.floodControlTable {
		if fce.IsStale(now) {
			delete(s.floodControlTable, key)
		}
	}
}

// NukeOldConnections 清理旧连接
func (s *Server) NukeOldConnections() {
	s.sessionsMutex.Lock()
	defer s.sessionsMutex.Unlock()

	now := time.Now()
	for _, session := range s.sessions {
		if session.DoTimeoutCheck(now) {
			util.Debug("将会话 %d 添加到超时终止队列", session.connID)
			session.ForceClose()
			delete(s.sessions, session.connID)
		}
	}
	s.sessionCount = len(s.sessions)
}

// GetBandwidthMonitor 获取带宽监控器
func (s *Server) GetBandwidthMonitor() *BandwidthMonitor {
	return s.bandwidthMonitor
}

// FloodControlEntry 洪水控制条目
type FloodControlEntry struct {
	addr         net.IP
	connectCount int
	lastConnect  time.Time
	blocktime    time.Time
}

// IsStale 检查是否过期
func (fce *FloodControlEntry) IsStale(now time.Time) bool {
	return fce.lastConnect.Before(now.Add(-60 * time.Second))
}

// IsBlocked 检查是否被阻止
func (fce *FloodControlEntry) IsBlocked() bool {
	return time.Now().Before(fce.blocktime)
}

// Hit 记录一次连接
func (fce *FloodControlEntry) Hit() bool {
	now := time.Now()
	elapsed := int(now.Sub(fce.lastConnect).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}

	// 衰减连接计数
	fce.connectCount = max(0, fce.connectCount-elapsed) + 1
	fce.lastConnect = now

	if fce.connectCount > 10 {
		// 阻止此客户端连接 60 秒
		fce.blocktime = now.Add(60 * time.Second)
		return false
	}

	return true
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
