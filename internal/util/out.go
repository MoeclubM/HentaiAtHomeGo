// Package util 提供日志输出功能
package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 日志级别常量
const (
	DEBUG = 1 << iota
	INFO
	WARNING
	ERROR
)

// 日志输出组合
const (
	LOGOUT = DEBUG | INFO | WARNING | ERROR
	LOGERR = WARNING | ERROR
	OUTPUT = INFO | WARNING | ERROR
	VERBOSE = ERROR
)

// OutListener 日志监听器接口
type OutListener interface {
	OutputWritten(entry string)
}

// Out 日志系统
type Out struct {
	mu                sync.RWMutex
	listeners         []OutListener
	suppressedOutput  int
	writeLogs         bool
	disableLogs       bool
	flushLogs         bool
	logoutCount       int
	logerrCount       int
	logoutPath        string
	logerrPath        string
	logoutFile        *os.File
	logerrFile        *os.File
	defOut            io.Writer
	defErr            io.Writer
}

var (
	globalOut *Out
	once      sync.Once
)

// GetOut 获取全局日志实例（单例）
func GetOut() *Out {
	once.Do(func() {
		globalOut = &Out{
			listeners:        make([]OutListener, 0),
			suppressedOutput: 0,
			writeLogs:        false,
			disableLogs:      false,
			flushLogs:        false,
			logoutCount:      0,
			logerrCount:      0,
			defOut:           os.Stdout,
			defErr:           os.Stderr,
		}
	})
	return globalOut
}

// StartLoggers 启动日志文件写入
func (o *Out) StartLoggers(logDir string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.logoutPath = filepath.Join(logDir, "log_out")
	o.logerrPath = filepath.Join(logDir, "log_err")

	// 启动错误日志
	if err := o.startLogger(&o.logerrFile, o.logerrPath, true); err != nil {
		return err
	}

	// 如果没有禁用日志，启动输出日志
	if !o.disableLogs {
		o.writeLogs = true
		return o.startLogger(&o.logoutFile, o.logoutPath, true)
	}

	return nil
}

// startLogger 启动单个日志文件
func (o *Out) startLogger(file **os.File, path string, writeHeader bool) error {
	// 删除旧日志文件
	oldPath := path + ".old"
	os.Remove(oldPath)

	// 重命名当前日志文件（如果存在）
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			os.Rename(path, oldPath)
		}
	}

	if path == "" {
		return nil
	}

	// 打开日志文件（追加模式）
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("无法打开日志文件 %s: %w", path, err)
	}

	*file = f

	if writeHeader {
		timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		o.logToFile(f, fmt.Sprintf("\n%s Logging started", timestamp), true)
	}

	return nil
}

// stopLogger 停止日志文件
func (o *Out) stopLogger(file **os.File) error {
	if *file == nil {
		return nil
	}

	if err := (*file).Close(); err != nil {
		return fmt.Errorf("无法关闭文件写入器: 无法轮换日志")
	}

	*file = nil
	return nil
}

// AddListener 添加日志监听器
func (o *Out) AddListener(listener OutListener) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, l := range o.listeners {
		if l == listener {
			return
		}
	}
	o.listeners = append(o.listeners, listener)
}

// RemoveListener 移除日志监听器
func (o *Out) RemoveListener(listener OutListener) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i, l := range o.listeners {
		if l == listener {
			o.listeners = append(o.listeners[:i], o.listeners[i+1:]...)
			return
		}
	}
}

// DisableLogging 禁用日志
func (o *Out) DisableLogging() {
	o.mu.Lock()
	if !o.writeLogs {
		o.mu.Unlock()
		return
	}

	o.writeLogs = false
	logoutFile := o.logoutFile
	o.logoutFile = nil
	o.mu.Unlock()

	if logoutFile != nil {
		_ = logoutFile.Sync()
		_ = logoutFile.Close()
	}

	Info("Logging ended.")
}

// FlushLogs 刷新日志缓冲区
func (o *Out) FlushLogs() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.logoutFile != nil {
		_ = o.logoutFile.Sync()
	}
	if o.logerrFile != nil {
		_ = o.logerrFile.Sync()
	}
}

// SetDisableLogs 设置是否禁用日志
func (o *Out) SetDisableLogs(disable bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.disableLogs = disable
}

// SetFlushLogs 设置是否刷新日志
func (o *Out) SetFlushLogs(flush bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.flushLogs = flush
}

// SetSuppressedOutput 设置抑制输出级别
func (o *Out) SetSuppressedOutput(level int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.suppressedOutput = level
}

// Debug 输出调试信息
func Debug(format string, args ...interface{}) {
	GetOut().log(DEBUG, "debug", format, args...)
}

// Info 输出信息
func Info(format string, args ...interface{}) {
	GetOut().log(INFO, "info", format, args...)
}

// Warning 输出警告
func Warning(format string, args ...interface{}) {
	GetOut().log(WARNING, "WARN", format, args...)
}

// Error 输出错误
func Error(format string, args ...interface{}) {
	GetOut().log(ERROR, "ERROR", format, args...)
}

// log 内部日志方法
func (o *Out) log(severity int, level, format string, args ...interface{}) {
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}

	// 检查是否需要输出
	output := (severity&OUTPUT & ^o.suppressedOutput) > 0
	log := (severity & (LOGOUT | LOGERR)) > 0

	if !output && !log {
		return
	}

	// 获取时间戳
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	// 获取调用位置
	verbose := o.getVerboseInfo(severity)

	// 分行处理
	lines := splitLines(message)
	for _, line := range lines {
		data := fmt.Sprintf("%s [%s] %s%s", timestamp, level, verbose, line)

		o.mu.Lock()

		if output {
			if severity&LOGERR != 0 {
				_, _ = fmt.Fprintln(o.defErr, data)
			} else {
				_, _ = fmt.Fprintln(o.defOut, data)
			}

			// 通知监听器
			for _, listener := range o.listeners {
				listener.OutputWritten(data)
			}
		}

		if log {
			if severity&LOGERR != 0 && o.logerrFile != nil {
				o.logToFile(o.logerrFile, data, true)
				o.logerrCount++
				if o.logerrCount > 10000 {
					o.logerrCount = 0
					o.rotateLoggerLocked(&o.logerrFile, o.logerrPath, false)
				}
			} else if o.writeLogs && o.logoutFile != nil {
				o.logToFile(o.logoutFile, data, false)
				o.logoutCount++
				if o.logoutCount > 100000 {
					o.logoutCount = 0
					o.rotateLoggerLocked(&o.logoutFile, o.logoutPath, false)
				}
			}
		}

		o.mu.Unlock()
	}
}

func (o *Out) rotateLoggerLocked(file **os.File, path string, writeHeader bool) {
	if err := o.stopLogger(file); err != nil {
		o.writeDirectError(err)
		return
	}
	if err := o.startLogger(file, path, writeHeader); err != nil {
		o.writeDirectError(err)
	}
}

func (o *Out) writeDirectError(err error) {
	if err == nil {
		return
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	_, _ = fmt.Fprintln(o.defErr, fmt.Sprintf("%s [ERROR] %s", timestamp, err.Error()))
}

// logToFile 写入日志文件
func (o *Out) logToFile(file *os.File, data string, flush bool) {
	if file == nil {
		return
	}

	_, _ = fmt.Fprintln(file, data)

	if flush || o.flushLogs {
		_ = file.Sync()
	}
}

// getVerboseInfo 获取详细的调用位置信息
func (o *Out) getVerboseInfo(severity int) string {
	if severity&VERBOSE == 0 {
		return ""
	}

	// 获取调用栈，跳过前几层
	pc, file, line, ok := runtime.Caller(3)
	if !ok {
		return "{Unknown Source} "
	}

	// 跳过内部调用
	for {
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			break
		}
		name := fn.Name()
		if !contains(name, "util.") && !contains(name, "runtime.") {
			break
		}
		pc, file, line, ok = runtime.Caller(4)
		if !ok {
			break
		}
	}

	if ok {
		return fmt.Sprintf("{%s:%d} ", filepath.Base(file), line)
	}

	return "{Unknown Source} "
}

// splitLines 分割字符串为行
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
