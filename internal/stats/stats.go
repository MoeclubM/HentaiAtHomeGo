// Package stats 提供统计功能
package stats

import (
	"sync"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
)

// StatListener 统计监听器接口
type StatListener interface {
	StatChanged(stat string)
}

// Stats 统计系统
type Stats struct {
	mu                sync.RWMutex
	statListeners     []StatListener
	statListenersLock sync.Mutex

	clientRunning   bool
	clientSuspended bool
	programStatus   string
	clientStartTime time.Time

	filesSent  int64
	filesRcvd  int64
	bytesSent  int64
	bytesRcvd  int64

	cacheCount     int
	cacheSize      int64
	bytesSentHistory []int // 361 个元素，每个代表 10 秒，共 1 小时
	openConnections int
	lastServerContact int
}

var (
	globalStats *Stats
	once        sync.Once
)

// GetStats 获取全局统计实例（单例）
func GetStats() *Stats {
	once.Do(func() {
		globalStats = &Stats{
			statListeners:     make([]StatListener, 0),
			bytesSentHistory:  make([]int, 361),
			programStatus:     "Stopped",
		}
	})
	return globalStats
}

// AddStatListener 添加统计监听器
func (s *Stats) AddStatListener(listener StatListener) {
	s.statListenersLock.Lock()
	defer s.statListenersLock.Unlock()

	for _, l := range s.statListeners {
		if l == listener {
			return
		}
	}
	s.statListeners = append(s.statListeners, listener)
}

// RemoveStatListener 移除统计监听器
func (s *Stats) RemoveStatListener(listener StatListener) {
	s.statListenersLock.Lock()
	defer s.statListenersLock.Unlock()

	for i, l := range s.statListeners {
		if l == listener {
			s.statListeners = append(s.statListeners[:i], s.statListeners[i+1:]...)
			return
		}
	}
}

// statChanged 统计变化通知
func (s *Stats) statChanged(stat string) {
	s.statListenersLock.Lock()
	defer s.statListenersLock.Unlock()

	for _, listener := range s.statListeners {
		listener.StatChanged(stat)
	}
}

// ResetStats 重置统计
func (s *Stats) ResetStats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clientRunning = false
	s.programStatus = "Stopped"
	s.clientStartTime = time.Time{}
	s.lastServerContact = 0
	s.filesSent = 0
	s.filesRcvd = 0
	s.bytesSent = 0
	s.bytesRcvd = 0
	s.cacheCount = 0
	s.cacheSize = 0

	for i := range s.bytesSentHistory {
		s.bytesSentHistory[i] = 0
	}

	s.statChanged("reset")
}

// TrackBytesSentHistory 跟踪发送字节历史
func (s *Stats) TrackBytesSentHistory() {
	s.bytesSentHistory = make([]int, 361)
}

// ShiftBytesSentHistory 移动发送字节历史（每 10 秒调用）
func (s *Stats) ShiftBytesSentHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := 360; i > 0; i-- {
		s.bytesSentHistory[i] = s.bytesSentHistory[i-1]
	}
	s.bytesSentHistory[0] = 0

	s.statChanged("bytesSentHistory")
}

// ResetBytesSentHistory 重置发送字节历史
func (s *Stats) ResetBytesSentHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.bytesSentHistory {
		s.bytesSentHistory[i] = 0
	}

	s.statChanged("bytesSentHistory")
}

// ProgramStarted 程序启动
func (s *Stats) ProgramStarted() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clientStartTime = time.Now()
	s.clientRunning = true
	s.programStatus = "Running"

	s.statChanged("clientRunning")
	s.statChanged("programStatus")
}

// ProgramSuspended 程序暂停
func (s *Stats) ProgramSuspended() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clientSuspended = true
	s.programStatus = "Suspended"

	s.statChanged("clientSuspended")
	s.statChanged("programStatus")
}

// ProgramResumed 程序恢复
func (s *Stats) ProgramResumed() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clientSuspended = false
	s.programStatus = "Running"

	s.statChanged("clientSuspended")
	s.statChanged("programStatus")
}

// ServerContact 服务器联系
func (s *Stats) ServerContact() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastServerContact = int(time.Now().Unix())
	s.statChanged("lastServerContact")
}

// FileSent 文件发送计数
func (s *Stats) FileSent() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.filesSent++
	s.statChanged("fileSent")
}

// FileRcvd 文件接收计数
func (s *Stats) FileRcvd() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.filesRcvd++
	s.statChanged("fileRcvd")
}

// BytesSent 字节发送计数
func (s *Stats) BytesSent(b int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.clientRunning {
		s.bytesSent += int64(b)
		s.bytesSentHistory[0] += b
		s.statChanged("bytesSent")
	}
}

// BytesRcvd 字节接收计数
func (s *Stats) BytesRcvd(b int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.clientRunning {
		s.bytesRcvd += int64(b)
		s.statChanged("bytesRcvd")
	}
}

// SetCacheCount 设置缓存数量
func (s *Stats) SetCacheCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cacheCount = count
	s.statChanged("cacheCount")
}

// SetCacheSize 设置缓存大小
func (s *Stats) SetCacheSize(size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cacheSize = size
	s.statChanged("cacheSize")
}

// SetOpenConnections 设置打开连接数
func (s *Stats) SetOpenConnections(conns int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.openConnections = conns
	s.statChanged("openConnections")
}

// SetProgramStatus 设置程序状态
func (s *Stats) SetProgramStatus(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.programStatus = status
	s.statChanged("programStatus")
}

// Getter 方法

func (s *Stats) IsClientRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientRunning
}

func (s *Stats) IsClientSuspended() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientSuspended
}

func (s *Stats) GetProgramStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.programStatus
}

func (s *Stats) GetUptime() int {
	return int(s.GetUptimeDouble())
}

func (s *Stats) GetUptimeDouble() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.clientRunning || s.clientStartTime.IsZero() {
		return 0
	}

	return time.Since(s.clientStartTime).Seconds()
}

func (s *Stats) GetFilesSent() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filesSent
}

func (s *Stats) GetFilesRcvd() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filesRcvd
}

func (s *Stats) GetBytesSent() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bytesSent
}

func (s *Stats) GetBytesRcvd() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bytesRcvd
}

func (s *Stats) GetBytesSentHistory() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bytesSentHistory
}

func (s *Stats) GetBytesSentPerSec() int {
	uptime := s.GetUptimeDouble()
	if uptime > 0 {
		return int(s.GetBytesSent() / int64(uptime))
	}
	return 0
}

func (s *Stats) GetBytesRcvdPerSec() int {
	uptime := s.GetUptimeDouble()
	if uptime > 0 {
		return int(s.GetBytesRcvd() / int64(uptime))
	}
	return 0
}

func (s *Stats) GetCacheCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cacheCount
}

func (s *Stats) GetCacheSize() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cacheSize
}

func (s *Stats) GetCacheFree() int64 {
	settings := config.GetSettings()
	return settings.GetDiskLimitBytes() - s.GetCacheSize()
}

func (s *Stats) GetCacheFill() float64 {
	settings := config.GetSettings()
	limit := settings.GetDiskLimitBytes()
	if limit == 0 {
		return 0
	}
	return float64(s.GetCacheSize()) / float64(limit)
}

func (s *Stats) GetOpenConnections() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.openConnections
}

func (s *Stats) GetLastServerContact() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastServerContact
}

// 全局便捷函数
func ResetStats() {
	GetStats().ResetStats()
}

func SetProgramStatus(status string) {
	GetStats().SetProgramStatus(status)
}

func ShiftBytesSentHistory() {
	GetStats().ShiftBytesSentHistory()
}

func ResetBytesSentHistory() {
	GetStats().ResetBytesSentHistory()
}

func ProgramStarted() {
	GetStats().ProgramStarted()
}

func ProgramSuspended() {
	GetStats().ProgramSuspended()
}

func ProgramResumed() {
	GetStats().ProgramResumed()
}

func ServerContact() {
	GetStats().ServerContact()
}

func FileSent() {
	GetStats().FileSent()
}

func FileRcvd() {
	GetStats().FileRcvd()
}

func BytesSent(b int) {
	GetStats().BytesSent(b)
}

func BytesRcvd(b int) {
	GetStats().BytesRcvd(b)
}

func SetCacheCount(count int) {
	GetStats().SetCacheCount(count)
}

func SetCacheSize(size int64) {
	GetStats().SetCacheSize(size)
}

func SetOpenConnections(conns int) {
	GetStats().SetOpenConnections(conns)
}

func GetOpenConnections() int {
	return GetStats().GetOpenConnections()
}
