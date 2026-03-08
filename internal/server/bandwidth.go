// Package server 提供带宽监控功能
package server

import (
	"sync"
	"time"
)

// BandwidthMonitor 带宽监控器
type BandwidthMonitor struct {
	mu                sync.Mutex
	quotaBytes        int
	lastQuotaReset    time.Time
	bytesPerSecond    int
}

// NewBandwidthMonitor 创建新的带宽监控器
func NewBandwidthMonitor() *BandwidthMonitor {
	return &BandwidthMonitor{
		lastQuotaReset: time.Now(),
	}
}

// SetBytesPerSecond 设置每秒字节数限制
func (bwm *BandwidthMonitor) SetBytesPerSecond(bps int) {
	bwm.mu.Lock()
	defer bwm.mu.Unlock()
	bwm.bytesPerSecond = bps
}

// WaitForQuota 等待配额
func (bwm *BandwidthMonitor) WaitForQuota(thread interface{}, bytesNeeded int) {
	if bytesNeeded <= 0 {
		return
	}

	bwm.mu.Lock()

	// 检查是否需要重置配额
	now := time.Now()
	elapsed := now.Sub(bwm.lastQuotaReset).Seconds()
	if elapsed >= 1.0 {
		// 每秒重置配额
		bwm.quotaBytes = bwm.bytesPerSecond
		bwm.lastQuotaReset = now
	}

	// 检查是否有足够的配额
	for bwm.quotaBytes < bytesNeeded {
		// 等待配额刷新
		bwm.mu.Unlock()

		// 计算需要等待的时间
		waitTime := time.Duration(float64(bytesNeeded-bwm.quotaBytes)/float64(bwm.bytesPerSecond)*1000) * time.Millisecond
		if waitTime > 100*time.Millisecond {
			waitTime = 100 * time.Millisecond
		}
		if waitTime < 10*time.Millisecond {
			waitTime = 10 * time.Millisecond
		}

		time.Sleep(waitTime)

		bwm.mu.Lock()

		// 重新检查配额
		now = time.Now()
		elapsed = now.Sub(bwm.lastQuotaReset).Seconds()
		if elapsed >= 1.0 {
			bwm.quotaBytes = bwm.bytesPerSecond
			bwm.lastQuotaReset = now
		}
	}

	// 扣除配额
	bwm.quotaBytes -= bytesNeeded
	bwm.mu.Unlock()
}
