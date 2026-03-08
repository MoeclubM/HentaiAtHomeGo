// Package hvfile 提供 HVFile 相关功能
package hvfile

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/util"
)

// HVFile 表示一个 H@H 文件
type HVFile struct {
	hash string
	size int
	xres int
	yres int
	typ  string
}

var (
	// 带分辨率的文件 ID 正则：hash-size-xres-yres-type
	fileIDWithResPattern = regexp.MustCompile(`^([a-f0-9]{40})-([0-9]{1,10})-([0-9]{1,5})-([0-9]{1,5})-(jpg|png|gif|mp4|wbm|wbp|avf|jxl)$`)
	// 不带分辨率的文件 ID 正则：hash-size-type
	fileIDWithoutResPattern = regexp.MustCompile(`^([a-f0-9]{40})-([0-9]{1,10})-(jpg|png|gif|mp4|wbm|wbp|avf|jxl)$`)
)

// NewHVFile 创建新的 HVFile
func NewHVFile(hash string, size, xres, yres int, typ string) *HVFile {
	return &HVFile{
		hash: hash,
		size: size,
		xres: xres,
		yres: yres,
		typ:  typ,
	}
}

// GetHVFileFromFileid 从文件 ID 创建 HVFile
func GetHVFileFromFileid(fileid string) (*HVFile, error) {
	// 尝试匹配带分辨率的格式
	matches := fileIDWithResPattern.FindStringSubmatch(fileid)
	if matches != nil {
		size, _ := strconv.Atoi(matches[2])
		xres, _ := strconv.Atoi(matches[3])
		yres, _ := strconv.Atoi(matches[4])
		typ := matches[5]

		return NewHVFile(matches[1], size, xres, yres, typ), nil
	}

	// 尝试匹配不带分辨率的格式
	matches = fileIDWithoutResPattern.FindStringSubmatch(fileid)
	if matches != nil {
		size, _ := strconv.Atoi(matches[2])
		typ := matches[3]

		return NewHVFile(matches[1], size, 0, 0, typ), nil
	}

	util.Warning("无效的文件 ID: \"%s\"", fileid)
	return nil, fmt.Errorf("无效的文件 ID: %s", fileid)
}

// GetHVFileFromFile 从文件创建 HVFile
func GetHVFileFromFile(filePath string) (*HVFile, error) {
	return GetHVFileFromFileWithValidator(filePath, nil)
}

// GetHVFileFromFileWithValidator 从文件创建 HVFile（带验证器）
func GetHVFileFromFileWithValidator(filePath string, validator FileValidator) (*HVFile, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, fmt.Errorf("路径是目录而非文件")
	}

	fileid := info.Name()

	hvFile, err := GetHVFileFromFileid(fileid)
	if err != nil {
		return nil, err
	}

	if info.Size() != int64(hvFile.size) {
		return nil, fmt.Errorf("文件大小不匹配")
	}

	if validator != nil {
		if !validator.ValidateFile(filePath, hvFile.hash) {
			return nil, fmt.Errorf("文件验证失败")
		}
	}

	return hvFile, nil
}

// FileValidator 文件验证器接口
type FileValidator interface {
	ValidateFile(filePath, hash string) bool
}

// GetLocalFileRef 获取本地文件引用路径
func (f *HVFile) GetLocalFileRef() string {
	settings := config.GetSettings()
	cacheDir := settings.GetCacheDir()

	// 路径格式：cache/hash[0:2]/hash[2:4]/fileid
	return fmt.Sprintf("%s/%s/%s/%s", cacheDir, f.hash[0:2], f.hash[2:4], f.GetFileID())
}

// GetMimeType 获取 MIME 类型
func (f *HVFile) GetMimeType() string {
	switch f.typ {
	case "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "mp4":
		return "video/mp4"
	case "wbm":
		return "video/webm"
	case "wbp":
		return "image/webp"
	case "avf":
		return "image/avif"
	case "jxl":
		return "image/jxl"
	default:
		return "application/octet-stream"
	}
}

// GetFileID 获取文件 ID
func (f *HVFile) GetFileID() string {
	if f.xres > 0 {
		return fmt.Sprintf("%s-%d-%d-%d-%s", f.hash, f.size, f.xres, f.yres, f.typ)
	}
	return fmt.Sprintf("%s-%d-%s", f.hash, f.size, f.typ)
}

// GetHash 获取哈希值
func (f *HVFile) GetHash() string {
	return f.hash
}

// GetSize 获取文件大小
func (f *HVFile) GetSize() int {
	return f.size
}

// GetType 获取文件类型
func (f *HVFile) GetType() string {
	return f.typ
}

// GetXRes 获取 X 分辨率
func (f *HVFile) GetXRes() int {
	return f.xres
}

// GetYRes 获取 Y 分辨率
func (f *HVFile) GetYRes() int {
	return f.yres
}

// GetStaticRange 获取静态范围（前 4 个字符）
func (f *HVFile) GetStaticRange() string {
	if len(f.hash) < 4 {
		return ""
	}
	return f.hash[0:4]
}

// IsValidHVFileID 检查是否是有效的 HVFile ID
func IsValidHVFileID(fileid string) bool {
	return fileIDWithResPattern.MatchString(fileid) || fileIDWithoutResPattern.MatchString(fileid)
}

// String 返回文件 ID 的字符串表示
func (f *HVFile) String() string {
	return f.GetFileID()
}
