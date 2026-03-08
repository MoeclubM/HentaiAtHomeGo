// Package config 提供配置管理功能
package config

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qwq/hentaiathomego/internal/util"
)

// 常量定义
const (
	CLIENT_BUILD        = 176
	CLIENT_KEY_LENGTH   = 20
	MAX_KEY_TIME_DRIFT  = 300
	MAX_CONNECTION_BASE = 20
	TCP_PACKET_SIZE     = 1460

	CLIENT_VERSION        = "1.6.4"
	CLIENT_RPC_PROTOCOL   = "http://"
	CLIENT_RPC_HOST       = "rpc.hentaiathome.net"
	CLIENT_LOGIN_FILENAME = "client_login"
	CONTENT_TYPE_DEFAULT  = "text/html; charset=iso-8859-1"
)

// Settings 配置管理
type Settings struct {
	mu sync.RWMutex

	// 客户端标识
	clientID   int
	clientKey  string
	clientHost string
	clientPort int

	// RPC 服务器
	rpcServerLock       sync.Mutex
	rpcServers          []net.IP
	rpcServerPort       int
	rpcServerCurrent    string
	rpcServerLastFailed string
	rpcPath             string

	// 静态范围
	staticRanges            map[string]int
	currentStaticRangeCount int

	// 目录配置
	dataDirPath     string
	logDirPath      string
	cacheDirPath    string
	tempDirPath     string
	downloadDirPath string

	// 服务器配置
	serverTimeDelta int

	// 限制配置
	throttleBytes       int
	diskLimitBytes      int64
	diskRemainingBytes  int64
	fileSystemBlockSize int64
	maxAllowedFileSize  int
	maxFilenameLength   int
	overrideConnections int

	// 代理配置
	imageProxyType string
	imageProxyHost string
	imageProxyPort int

	// 布尔配置
	verifyCache             bool
	rescanCache             bool
	skipFreeSpaceCheck      bool
	warnNewClient           bool
	useLessMemory           bool
	disableBWM              bool
	disableDownloadBWM      bool
	disableFileVerification bool
	disableLogs             bool
	flushLogs               bool
	disableIPOriginCheck    bool
	disableFloodControl     bool
}

var (
	globalSettings *Settings
	once           sync.Once
)

// GetSettings 获取全局配置实例（单例）
func GetSettings() *Settings {
	once.Do(func() {
		globalSettings = &Settings{
			clientID:                0,
			clientKey:               "",
			clientHost:              "",
			clientPort:              0,
			rpcServerPort:           80,
			rpcPath:                 "15/rpc?",
			staticRanges:            make(map[string]int),
			currentStaticRangeCount: 0,
			dataDirPath:             "data",
			logDirPath:              "log",
			cacheDirPath:            "cache",
			tempDirPath:             "tmp",
			downloadDirPath:         "download",
			serverTimeDelta:         0,
			throttleBytes:           0,
			diskLimitBytes:          0,
			diskRemainingBytes:      0,
			fileSystemBlockSize:     4096,
			maxAllowedFileSize:      1073741824, // 1GB
			maxFilenameLength:       125,
			overrideConnections:     0,
			imageProxyPort:          0,
			verifyCache:             false,
			rescanCache:             false,
			skipFreeSpaceCheck:      false,
			warnNewClient:           false,
			useLessMemory:           false,
			disableBWM:              false,
			disableDownloadBWM:      false,
			disableFileVerification: false,
			disableLogs:             false,
			flushLogs:               false,
			disableIPOriginCheck:    false,
			disableFloodControl:     false,
		}
	})
	return globalSettings
}

// InitializeDirectories 初始化目录
func (s *Settings) InitializeDirectories() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	util.Debug("Using --data-dir=%s", s.dataDirPath)
	if err := util.CheckAndCreateDir(s.dataDirPath); err != nil {
		return err
	}

	util.Debug("Using --log-dir=%s", s.logDirPath)
	if err := util.CheckAndCreateDir(s.logDirPath); err != nil {
		return err
	}

	util.Debug("Using --cache-dir=%s", s.cacheDirPath)
	if err := util.CheckAndCreateDir(s.cacheDirPath); err != nil {
		return err
	}

	util.Debug("Using --temp-dir=%s", s.tempDirPath)
	if err := util.CheckAndCreateDir(s.tempDirPath); err != nil {
		return err
	}

	util.Debug("Using --download-dir=%s", s.downloadDirPath)
	if err := util.CheckAndCreateDir(s.downloadDirPath); err != nil {
		return err
	}

	return nil
}

// LoginCredentialsAreSyntaxValid 检查登录凭证语法是否有效
func (s *Settings) LoginCredentialsAreSyntaxValid() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.clientID < 1000 {
		return false
	}

	matched, _ := regexp.MatchString("^[a-zA-Z0-9]{"+strconv.Itoa(CLIENT_KEY_LENGTH)+"}$", s.clientKey)
	return matched
}

// LoadClientLoginFromFile 从文件加载客户端登录信息
func (s *Settings) LoadClientLoginFromFile() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	loginPath := filepath.Join(s.dataDirPath, CLIENT_LOGIN_FILENAME)

	if !util.FileExists(loginPath) {
		return false
	}

	content, err := util.GetStringFileContents(loginPath)
	if err != nil {
		util.Warning("读取 %s 时出错: %v", CLIENT_LOGIN_FILENAME, err)
		return false
	}

	if content == "" {
		return false
	}

	parts := strings.SplitN(content, "-", 2)
	if len(parts) != 2 {
		return false
	}

	id, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return false
	}

	s.clientID = id
	s.clientKey = strings.TrimSpace(parts[1])
	util.Info("从 %s 加载登录设置", CLIENT_LOGIN_FILENAME)

	return true
}

// PromptForIDAndKey 提示用户输入 ID 和密钥
func (s *Settings) PromptForIDAndKey(handler util.InputQueryHandler) error {
	util.Info("在使用此客户端之前，您需要在 https://e-hentai.org/hentaiathome.php 注册")
	util.Info("重要：您想运行的每个客户端都需要单独的标识符")
	util.Info("不要输入已分配给其他客户端的标识符，除非它已被停用")
	util.Info("注册后，输入您的 ID 和密钥以启动客户端")
	util.Info("(只需执行一次)\n")

	s.mu.Lock()
	s.clientID = 0
	s.clientKey = ""
	s.mu.Unlock()

	// 获取客户端 ID
	for {
		query, err := handler.QueryString("输入客户端 ID")
		if err != nil {
			return err
		}

		id, err := strconv.Atoi(strings.TrimSpace(query))
		if err != nil || id < 1000 {
			util.Warning("无效的客户端 ID，请重试")
			continue
		}

		s.mu.Lock()
		s.clientID = id
		s.mu.Unlock()
		break
	}

	// 获取客户端密钥
	for {
		query, err := handler.QueryString("输入客户端密钥")
		if err != nil {
			return err
		}

		key := strings.TrimSpace(query)

		s.mu.Lock()
		s.clientKey = key
		s.mu.Unlock()

		if !s.LoginCredentialsAreSyntaxValid() {
			util.Warning("无效的客户端密钥，必须恰好是 20 个字母数字字符，请重试")
			continue
		}

		break
	}

	// 保存到文件
	loginPath := filepath.Join(s.dataDirPath, CLIENT_LOGIN_FILENAME)
	content := fmt.Sprintf("%d-%s", s.clientID, s.clientKey)
	if err := util.PutStringFileContents(loginPath, content); err != nil {
		util.Warning("写入 %s 时出错: %v", CLIENT_LOGIN_FILENAME, err)
	}

	return nil
}

// ParseAndUpdateSettings 解析并更新设置
func (s *Settings) ParseAndUpdateSettings(settings []string) bool {
	if settings == nil {
		return false
	}

	for _, setting := range settings {
		if setting == "" {
			continue
		}

		parts := strings.SplitN(setting, "=", 2)
		if len(parts) == 2 {
			s.updateSetting(parts[0], parts[1])
		}
	}

	return true
}

// ParseArgs 解析命令行参数
func (s *Settings) ParseArgs(args []string) bool {
	if args == nil {
		return false
	}

	allowedArgs := map[string]bool{
		"data_dir":     true,
		"log_dir":      true,
		"cache_dir":    true,
		"temp_dir":     true,
		"download_dir": true,
	}

	for _, arg := range args {
		if arg == "" {
			continue
		}

		if !strings.HasPrefix(arg, "--") {
			util.Warning("无效的命令参数: %s", arg)
			continue
		}

		arg = arg[2:] // 移除 --

		parts := strings.SplitN(arg, "=", 2)
		key := strings.ReplaceAll(parts[0], "-", "_")
		key = strings.ToLower(key)

		if !allowedArgs[key] {
			util.Warning("忽略本地参数 %s，运行参数应由管理端下发", parts[0])
			continue
		}

		if len(parts) == 2 {
			s.updateSetting(key, parts[1])
		} else {
			s.updateSetting(key, "true")
		}
	}

	return true
}

// updateSetting 更新单个设置
func (s *Settings) updateSetting(setting, value string) bool {
	setting = strings.ReplaceAll(setting, "-", "_")
	setting = strings.ToLower(setting)

	s.mu.Lock()
	defer s.mu.Unlock()

	switch setting {
	case "min_client_build":
		build, err := strconv.Atoi(value)
		if err == nil && build > CLIENT_BUILD {
			util.Error("您的客户端版本过旧，无法连接到 Hentai@Home 网络。请从 http://hentaiathome.net/ 下载新版本")
			return false
		}

	case "cur_client_build":
		build, err := strconv.Atoi(value)
		if err == nil && build > CLIENT_BUILD {
			s.warnNewClient = true
		}

	case "server_time":
		serverTime, err := strconv.Atoi(value)
		if err == nil {
			s.serverTimeDelta = serverTime - int(getCurrentTimeSeconds())
			util.Debug("设置已更改: serverTimeDelta=%d", s.serverTimeDelta)
		}

	case "rpc_server_port":
		port, err := strconv.ParseInt(value, 10, 16)
		if err == nil {
			s.rpcServerPort = int(port)
		}

	case "rpc_server_ip":
		s.rpcServerLock.Lock()
		ips := strings.Split(value, ";")
		s.rpcServers = make([]net.IP, 0, len(ips))
		keepCurrent := false

		for _, ipStr := range ips {
			ip := net.ParseIP(strings.TrimSpace(ipStr))
			if ip != nil {
				s.rpcServers = append(s.rpcServers, ip)

				if s.rpcServerCurrent != "" && ip.String() == s.rpcServerCurrent {
					keepCurrent = true
				}
			}
		}

		if !keepCurrent {
			util.Debug("不保留当前 rpcServerCurrent")
			s.rpcServerCurrent = ""
		} else {
			util.Debug("保留当前 rpcServerCurrent=%s", s.rpcServerCurrent)
		}
		s.rpcServerLock.Unlock()

	case "rpc_path":
		s.rpcPath = value

	case "host":
		s.clientHost = value

	case "port":
		if s.clientPort == 0 {
			port, err := strconv.Atoi(value)
			if err == nil {
				s.clientPort = port
			}
		}

	case "throttle_bytes":
		throttle, err := strconv.Atoi(value)
		if err == nil {
			s.throttleBytes = throttle
		}

	case "disklimit_bytes":
		limit, err := strconv.ParseInt(value, 10, 64)
		if err == nil && limit >= s.diskLimitBytes {
			s.diskLimitBytes = limit
		}

	case "diskremaining_bytes":
		remaining, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			s.diskRemainingBytes = remaining
		}

	case "filesystem_blocksize":
		blocksize, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			if blocksize < 0 || blocksize > 65536 {
				util.Warning("文件系统块大小 %d 字节不合理，使用默认值 4096 字节", blocksize)
				s.fileSystemBlockSize = 4096
			} else {
				s.fileSystemBlockSize = blocksize
			}
		}

	case "rescan_cache":
		s.rescanCache = value == "true"

	case "verify_cache":
		s.verifyCache = value == "true"
		s.rescanCache = value == "true"

	case "use_less_memory":
		s.useLessMemory = value == "true"

	case "disable_logging":
		s.disableLogs = value == "true"

	case "disable_bwm":
		s.disableBWM = value == "true"
		s.disableDownloadBWM = value == "true"

	case "disable_download_bwm":
		s.disableDownloadBWM = value == "true"

	case "disable_file_verification":
		s.disableFileVerification = value == "true"

	case "disable_ip_origin_check":
		s.disableIPOriginCheck = value == "true"

	case "disable_flood_control":
		s.disableFloodControl = value == "true"

	case "skip_free_space_check":
		s.skipFreeSpaceCheck = value == "true"

	case "max_connections":
		conns, err := strconv.Atoi(value)
		if err == nil {
			s.overrideConnections = conns
		}

	case "max_allowed_filesize":
		size, err := strconv.Atoi(value)
		if err == nil {
			s.maxAllowedFileSize = size
		}

	case "max_filename_length":
		length, err := strconv.Atoi(value)
		if err == nil {
			s.maxFilenameLength = length
		}

	case "static_ranges":
		// 静态范围在启动时发送
		s.staticRanges = make(map[string]int)
		s.currentStaticRangeCount = 0

		ranges := strings.Split(value, ";")
		for _, r := range ranges {
			if len(r) == 4 {
				s.currentStaticRangeCount++
				s.staticRanges[r] = 1
			}
		}

	case "static_range_count":
		count, err := strconv.Atoi(value)
		if err == nil {
			s.currentStaticRangeCount = count
		}

	case "cache_dir":
		s.cacheDirPath = value

	case "temp_dir":
		s.tempDirPath = value

	case "data_dir":
		s.dataDirPath = value

	case "log_dir":
		s.logDirPath = value

	case "download_dir":
		s.downloadDirPath = value

	case "image_proxy_type":
		s.imageProxyType = strings.ToLower(value)

	case "image_proxy_host":
		s.imageProxyHost = strings.ToLower(value)

	case "image_proxy_port":
		port, err := strconv.Atoi(value)
		if err == nil {
			s.imageProxyPort = port
		}

	case "flush_logs":
		s.flushLogs = value == "true"

	case "silentstart":
		// 由 GUI 处理，忽略

	default:
		util.Warning("未知设置 %s = %s", setting, value)
		return false
	}

	util.Debug("设置已更改: %s=%s", setting, value)
	return true
}

// Getter 方法

func (s *Settings) GetClientID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientID
}

func (s *Settings) GetClientKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientKey
}

func (s *Settings) GetClientHost() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientHost
}

func (s *Settings) GetClientPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientPort
}

func (s *Settings) GetDataDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dataDirPath
}

func (s *Settings) GetLogDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logDirPath
}

func (s *Settings) GetCacheDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cacheDirPath
}

func (s *Settings) GetTempDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tempDirPath
}

func (s *Settings) GetDownloadDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.downloadDirPath
}

func (s *Settings) GetOutputLogPath() string {
	return filepath.Join(s.GetLogDir(), "log_out")
}

func (s *Settings) GetErrorLogPath() string {
	return filepath.Join(s.GetLogDir(), "log_err")
}

func (s *Settings) GetServerTime() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int(getCurrentTimeSeconds()) + s.serverTimeDelta
}

func (s *Settings) GetServerTimeDelta() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serverTimeDelta
}

func (s *Settings) GetThrottleBytesPerSec() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.throttleBytes
}

func (s *Settings) GetMaxAllowedFileSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxAllowedFileSize
}

func (s *Settings) GetDiskLimitBytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.diskLimitBytes
}

func (s *Settings) GetDiskMinRemainingBytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.diskRemainingBytes
}

func (s *Settings) GetFileSystemBlockSize() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fileSystemBlockSize
}

func (s *Settings) GetMaxFilenameLength() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxFilenameLength
}

func (s *Settings) GetRPCPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rpcPath
}

func (s *Settings) GetMaxConnections() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.overrideConnections > 0 {
		return s.overrideConnections
	}
	// throttle_bytes 在几年前被更改为必需值
	return MAX_CONNECTION_BASE + min(480, s.throttleBytes/10000)
}

func (s *Settings) IsStaticRange(fileID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(fileID) < 4 {
		return false
	}

	if s.staticRanges != nil {
		_, exists := s.staticRanges[fileID[:4]]
		return exists
	}

	return false
}

func (s *Settings) GetStaticRangeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentStaticRangeCount
}

func (s *Settings) IsVerifyCache() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.verifyCache
}

func (s *Settings) IsRescanCache() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rescanCache
}

func (s *Settings) IsUseLessMemory() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.useLessMemory
}

func (s *Settings) IsSkipFreeSpaceCheck() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skipFreeSpaceCheck
}

func (s *Settings) IsWarnNewClient() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.warnNewClient
}

func (s *Settings) IsDisableBWM() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disableBWM
}

func (s *Settings) IsDisableDownloadBWM() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disableDownloadBWM
}

func (s *Settings) IsDisableFileVerification() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disableFileVerification
}

func (s *Settings) IsDisableLogs() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disableLogs
}

func (s *Settings) IsFlushLogs() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.flushLogs
}

func (s *Settings) IsDisableIPOriginCheck() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disableIPOriginCheck
}

func (s *Settings) IsDisableFloodControl() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.disableFloodControl
}

func (s *Settings) IsImageProxyEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.imageProxyHost != ""
}

func (s *Settings) GetImageProxyHost() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.imageProxyHost
}

func (s *Settings) GetImageProxyType() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.imageProxyType == "" {
		return "socks"
	}
	return s.imageProxyType
}

func (s *Settings) GetImageProxyPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.imageProxyPort == 0 {
		if s.GetImageProxyType() == "socks" {
			return 1080
		}
		if s.GetImageProxyType() == "http" {
			return 8080
		}
	}
	return s.imageProxyPort
}

// IsValidRPCServer 检查是否是有效的 RPC 服务器
func (s *Settings) IsValidRPCServer(ip net.IP) bool {
	if s.IsDisableIPOriginCheck() {
		return true
	}

	s.rpcServerLock.Lock()
	defer s.rpcServerLock.Unlock()

	if s.rpcServers == nil {
		return false
	}

	for _, rpcIP := range s.rpcServers {
		if rpcIP.Equal(ip) {
			return true
		}
	}

	return false
}

// GetRPCServerHost 获取 RPC 服务器主机
func (s *Settings) GetRPCServerHost() string {
	s.rpcServerLock.Lock()
	defer s.rpcServerLock.Unlock()

	if s.rpcServerCurrent == "" {
		if s.rpcServers == nil || len(s.rpcServers) == 0 {
			return CLIENT_RPC_HOST
		}

		if len(s.rpcServers) == 1 {
			s.rpcServerCurrent = strings.ToLower(s.rpcServers[0].String())
		} else {
			idx := randomInt(len(s.rpcServers))
			scanDirection := 1
			if randomInt(2) == 0 {
				scanDirection = -1
			}

			for attempts := 0; attempts < len(s.rpcServers); attempts++ {
				candidate := strings.ToLower(s.rpcServers[(len(s.rpcServers)+idx)%len(s.rpcServers)].String())

				if s.rpcServerLastFailed != "" && candidate == s.rpcServerLastFailed {
					util.Debug("%s was marked as last failed", s.rpcServerLastFailed)
					idx += scanDirection
					continue
				}

				s.rpcServerCurrent = candidate
				util.Debug("选择 rpcServerCurrent=%s", s.rpcServerCurrent)
				break
			}

			if s.rpcServerCurrent == "" {
				fallbackIdx := (len(s.rpcServers) + (idx % len(s.rpcServers))) % len(s.rpcServers)
				s.rpcServerCurrent = strings.ToLower(s.rpcServers[fallbackIdx].String())
			}
		}
	}

	if s.rpcServerPort == 80 {
		return s.rpcServerCurrent
	}
	return fmt.Sprintf("%s:%d", s.rpcServerCurrent, s.rpcServerPort)
}

// ClearRPCServerFailure 清除 RPC 服务器失败标记
func (s *Settings) ClearRPCServerFailure() {
	s.rpcServerLock.Lock()
	defer s.rpcServerLock.Unlock()

	if s.rpcServerLastFailed != "" {
		util.Debug("清除 rpcServerLastFailed")
		s.rpcServerLastFailed = ""
		s.rpcServerCurrent = ""
	}
}

// MarkRPCServerFailure 标记 RPC 服务器失败
func (s *Settings) MarkRPCServerFailure(host string) {
	s.rpcServerLock.Lock()
	defer s.rpcServerLock.Unlock()

	if s.rpcServerCurrent != "" {
		util.Debug("标记 %s 为 rpcServerLastFailed", host)
		s.rpcServerLastFailed = host
		s.rpcServerCurrent = ""
	}
}

// getCurrentTimeSeconds 获取当前时间（秒）
func getCurrentTimeSeconds() int64 {
	return time.Now().Unix()
}

// randomInt 生成随机整数
func randomInt(max int) int {
	if max <= 1 {
		return 0
	}
	return util.RandomInt(max)
}

// min 返回最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
