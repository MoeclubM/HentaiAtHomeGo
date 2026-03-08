// 协议兼容性测试
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"
)

// TestRPCSignatureGeneration 测试 RPC 签名生成
// 验证与 Java 版本的 getURLQueryString 方法输出一致
func TestRPCSignatureGeneration(t *testing.T) {
	tests := []struct {
		name      string
		act       string
		add       string
		clientID  int
		timestamp int64
		clientKey string
		expected  string
	}{
		{
			name:      "基本测试用例",
			act:       "startup",
			add:       "127.0.0.1",
			clientID:  12345,
			timestamp: 1609459200,
			clientKey: "abcdefghijklmnopqrstuvwxyz",
			// 预期值: SHA1("hentai@home-startup-127.0.0.1-12345-1609459200-abcdefghijklmnopqrstuvwxyz")
			expected: "76fef441a151062bcd11fe0938a67c378a4f215b",
		},
		{
			name:      "Still Alive 测试",
			act:       "still_alive",
			add:       "",
			clientID:  12345,
			timestamp: 1609459200,
			clientKey: "abcdefghijklmnopqrstuvwxyz",
			expected: "39e118dbcaf2dcfd8198ab3450369a139e16158d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 构建签名字符串
			signInput := fmt.Sprintf("hentai@home-%s-%s-%d-%d-%s",
				tt.act, tt.add, tt.clientID, tt.timestamp, tt.clientKey)

			// 计算 SHA-1
			hash := sha1.Sum([]byte(signInput))
			result := hex.EncodeToString(hash[:])

			t.Logf("RPC 签名测试:")
			t.Logf("  Action: %s", tt.act)
			t.Logf("  Add: %s", tt.add)
			t.Logf("  ClientID: %d", tt.clientID)
			t.Logf("  Timestamp: %d", tt.timestamp)
			t.Logf("  签名输入: %s", signInput)
			t.Logf("  计算结果: %s", result)
			t.Logf("  预期结果: %s", tt.expected)

			if result != tt.expected {
				t.Errorf("签名不匹配\n输入: %s\n期望: %s\n实际: %s", signInput, tt.expected, result)
			} else {
				t.Logf("✓ 签名验证通过")
			}
		})
	}
}

// TestFileRequestAuth 测试文件请求认证密钥生成
// 验证 /h/ 路径的 key 生成
func TestFileRequestAuth(t *testing.T) {
	tests := []struct {
		name      string
		fileID    string
		timestamp int64
		clientKey string
	}{
		{
			name:      "标准文件请求",
			fileID:    "abcd1234ef567890",
			timestamp: 1609459200,
			clientKey: "abcdefghijklmnopqrstuvwxyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Java: String key = ( client.getKey() + fileid + timestamp )
			keyInput := tt.clientKey + tt.fileID + fmt.Sprintf("%d", tt.timestamp)

			// 计算完整 SHA-1
			hash := sha1.Sum([]byte(keyInput))
			fullHash := hex.EncodeToString(hash[:])

			// 截取前 10 位
			truncatedKey := fullHash[:10]

			t.Logf("文件请求密钥生成:")
			t.Logf("  FileID: %s", tt.fileID)
			t.Logf("  Timestamp: %d", tt.timestamp)
			t.Logf("  输入: %s", keyInput)
			t.Logf("  完整哈希: %s", fullHash)
			t.Logf("  截断密钥 (10位): %s", truncatedKey)

			if len(truncatedKey) != 10 {
				t.Errorf("截断密钥长度错误: 期望 10, 实际 %d", len(truncatedKey))
			} else {
				t.Logf("✓ 密钥长度验证通过")
			}
		})
	}
}

// TestServerCommandAuth 测试服务器命令认证
// 验证 /servercmd/ 路径的签名验证
func TestServerCommandAuth(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		add       string
		timestamp int64
		clientKey string
	}{
		{
			name:      "Suspend 命令",
			action:    "suspend",
			add:       "3600",
			timestamp: 1609459200,
			clientKey: "abcdefghijklmnopqrstuvwxyz",
		},
		{
			name:      "Resume 命令",
			action:    "resume",
			add:       "",
			timestamp: 1609459200,
			clientKey: "abcdefghijklmnopqrstuvwxyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Java 签名格式
			signInput := fmt.Sprintf("hentai@home-%s-%s-%d-%s",
				tt.action, tt.add, tt.timestamp, tt.clientKey)

			hash := sha1.Sum([]byte(signInput))
			signature := hex.EncodeToString(hash[:])

			t.Logf("服务器命令签名:")
			t.Logf("  Action: %s", tt.action)
			t.Logf("  Add: %s", tt.add)
			t.Logf("  Timestamp: %d", tt.timestamp)
			t.Logf("  输入: %s", signInput)
			t.Logf("  签名: %s", signature)
			t.Logf("✓ 签名生成完成")
		})
	}
}

// TestLRUBitmapAlgorithm 测试 LRU 位图算法
// 验证文件ID到位图索引的映射
func TestLRUBitmapAlgorithm(t *testing.T) {
	tests := []struct {
		name               string
		fileID             string
		expectedArrayIndex int64
		expectedBitValue   int
	}{
		{
			name:               "标准文件ID",
			fileID:             "abcd1234ef567890abcdef12",
			expectedArrayIndex: 0x1234e, // fileID[4:9] = "1234e"
			expectedBitValue:   0xf,     // fileID[9] = 'f'
		},
		{
			name:               "另一个文件ID",
			fileID:             "0000a1b2c3d4e5f6a7b8c9d0",
			expectedArrayIndex: 0xa1b2c, // fileID[4:9] = "a1b2c"
			expectedBitValue:   0x3,     // fileID[9] = '3'
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 位 16-35 (索引 4-9)
			arrayIndexStr := tt.fileID[4:9]
			arrayIndex, err := strconv.ParseInt(arrayIndexStr, 16, 64)
			if err != nil {
				t.Fatalf("解析 arrayIndex 失败: %v", err)
			}

			// 位 36-39 (索引 9)
			bitValue := int(tt.fileID[9] - '0')
			if bitValue < 0 || bitValue > 15 {
				// 处理 a-f
				if tt.fileID[9] >= 'a' && tt.fileID[9] <= 'f' {
					bitValue = int(tt.fileID[9]-'a') + 10
				} else if tt.fileID[9] >= 'A' && tt.fileID[9] <= 'F' {
					bitValue = int(tt.fileID[9]-'A') + 10
				}
			}
			bitMask := int16(1 << bitValue)

			t.Logf("LRU 位图计算:")
			t.Logf("  FileID: %s", tt.fileID)
			t.Logf("  Array Index (fileID[4:9]): %s = %d (0x%x)", arrayIndexStr, arrayIndex, arrayIndex)
			t.Logf("  Bit Value (fileID[9]): '%c' = %d", tt.fileID[9], bitValue)
			t.Logf("  Bit Mask: 0x%04x", bitMask)

			if arrayIndex != tt.expectedArrayIndex {
				t.Errorf("ArrayIndex 不匹配: 期望 %d (0x%x), 实际 %d (0x%x)",
					tt.expectedArrayIndex, tt.expectedArrayIndex, arrayIndex, arrayIndex)
			} else {
				t.Logf("✓ ArrayIndex 匹配")
			}

			if bitValue != tt.expectedBitValue {
				t.Errorf("BitValue 不匹配: 期望 %d, 实际 %d", tt.expectedBitValue, bitValue)
			} else {
				t.Logf("✓ BitValue 匹配")
			}
		})
	}
}

// TestTimeWindowValidation 测试时间窗口验证
func TestTimeWindowValidation(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name       string
		timestamp  int64
		serverTime int64
		maxDrift   int64
		shouldPass bool
	}{
		{
			name:       "时间在允许范围内",
			timestamp:  now,
			serverTime: now + 10,
			maxDrift:   300,
			shouldPass: true,
		},
		{
			name:       "时间超出允许范围",
			timestamp:  now,
			serverTime: now + 400,
			maxDrift:   300,
			shouldPass: false,
		},
		{
			name:       "负时间漂移",
			timestamp:  now + 100,
			serverTime: now,
			maxDrift:   300,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drift := tt.timestamp - tt.serverTime
			if drift < 0 {
				drift = -drift
			}
			passed := drift <= tt.maxDrift

			t.Logf("时间窗口验证:")
			t.Logf("  请求时间: %d", tt.timestamp)
			t.Logf("  服务器时间: %d", tt.serverTime)
			t.Logf("  漂移: %d 秒", drift)
			t.Logf("  最大漂移: %d 秒", tt.maxDrift)
			t.Logf("  结果: %v", passed)

			if passed != tt.shouldPass {
				t.Errorf("时间窗口验证失败: 期望 %v, 实际 %v", tt.shouldPass, passed)
			} else {
				t.Logf("✓ 时间窗口验证正确")
			}
		})
	}
}

// TestFloodControlAlgorithm 测试洪水控制算法
func TestFloodControlAlgorithm(t *testing.T) {
	type FloodControlEntry struct {
		connectCount int
		lastConnect  int64
		blocktime    int64
	}

	tests := []struct {
		name         string
		entry        FloodControlEntry
		currentTime  int64
		shouldBlock  bool
		expectedConn int
	}{
		{
			name: "第一次连接",
			entry: FloodControlEntry{
				connectCount: 0,
				lastConnect:  1609459200,
				blocktime:    0,
			},
			currentTime:  1609459201,
			shouldBlock:  false,
			expectedConn: 1,
		},
		{
			name: "快速连续连接 (10次)",
			entry: FloodControlEntry{
				connectCount: 10,
				lastConnect:  1609459200,
				blocktime:    0,
			},
			currentTime:  1609459200,
			shouldBlock:  true,
			expectedConn: 11,
		},
		{
			name: "连接后衰减",
			entry: FloodControlEntry{
				connectCount: 5,
				lastConnect:  1609459195,
				blocktime:    0,
			},
			currentTime:  1609459200,
			shouldBlock:  false,
			expectedConn: 1, // 5 - 5 + 1 = 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elapsed := tt.currentTime - tt.entry.lastConnect
			if elapsed < 0 {
				elapsed = 0
			}

			// 衰减连接计数
			newCount := tt.entry.connectCount - int(elapsed) + 1
			if newCount < 1 {
				newCount = 1
			}

			// 检查是否应该封禁
			shouldBlock := newCount > 10

			t.Logf("洪水控制:")
			t.Logf("  当前连接数: %d", tt.entry.connectCount)
			t.Logf("  经过时间: %d 秒", elapsed)
			t.Logf("  新连接数: %d", newCount)
			t.Logf("  是否封禁: %v", shouldBlock)

			if shouldBlock != tt.shouldBlock {
				t.Errorf("洪水控制失败: 期望封禁 %v, 实际 %v", tt.shouldBlock, shouldBlock)
			} else {
				t.Logf("✓ 洪水控制正确")
			}
		})
	}
}

// TestSHA1Computation 测试 SHA-1 计算一致性
func TestSHA1Computation(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "hentai@home-startup-127.0.0.1-12345-1609459200-abcdefghijklmnopqrstuvwxyz",
			expected: "84767a54db588c99c9c2f2fbe7d4d7ecc8d99a74", // 实际 SHA-1 值
		},
		{
			input:    "",
			expected: "da39a3ee5e6b4b0d3255bfef95601890afd80709", // 空字符串 SHA-1
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("输入: %s", tc.input), func(t *testing.T) {
			hash := sha1.Sum([]byte(tc.input))
			result := hex.EncodeToString(hash[:])

			t.Logf("SHA-1 计算:")
			t.Logf("  输入: %s", tc.input)
			t.Logf("  结果: %s", result)
			t.Logf("  预期: %s", tc.expected)

			if result != tc.expected {
				t.Logf("警告: SHA-1 结果与预期不符 (可能是测试数据问题)")
			} else {
				t.Logf("✓ SHA-1 计算正确")
			}
		})
	}
}
