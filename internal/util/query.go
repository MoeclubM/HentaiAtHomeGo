// Package util 提供用户输入查询接口
package util

// InputQueryHandler 用户输入查询接口
// 用于查询用户信息（如首次启动时的客户端 ID 和密钥）
type InputQueryHandler interface {
	// QueryString 查询字符串输入
	QueryString(queryText string) (string, error)
}
