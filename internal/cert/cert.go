// Package cert 提供证书管理功能
package cert

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/download"
	"github.com/qwq/hentaiathomego/internal/util"
	"software.sslmate.com/src/go-pkcs12"
)

// CertificateHandler 证书处理器
type CertificateHandler struct {
	certFile   string
	certPass   string
	certExpiry time.Time
}

// NewCertificateHandler 创建新的证书处理器
func NewCertificateHandler() *CertificateHandler {
	settings := config.GetSettings()

	return &CertificateHandler{
		certFile: filepath.Join(settings.GetDataDir(), "hathcert.p12"),
		certPass: settings.GetClientKey(),
	}
}

// LoadCertificate 加载证书
func (ch *CertificateHandler) LoadCertificate() (*tls.Certificate, error) {
	if _, err := os.Stat(ch.certFile); os.IsNotExist(err) {
		if err := ch.downloadCertificate(); err != nil {
			return nil, fmt.Errorf("下载证书失败: %w", err)
		}
	}

	cert, err := ch.loadPKCS12Certificate()
	if err != nil {
		return nil, fmt.Errorf("加载证书失败: %w", err)
	}

	return &cert, nil
}

func (ch *CertificateHandler) loadPKCS12Certificate() (tls.Certificate, error) {
	pfxBytes, err := os.ReadFile(ch.certFile)
	if err != nil {
		return tls.Certificate{}, err
	}

	privKey, cert, caCerts, err := pkcs12.DecodeChain(pfxBytes, ch.certPass)
	if err != nil {
		return tls.Certificate{}, err
	}

	ch.certExpiry = cert.NotAfter
	util.Debug("证书过期时间: %v", ch.certExpiry)

	chain := make([][]byte, 0, 1+len(caCerts))
	chain = append(chain, cert.Raw)
	for _, caCert := range caCerts {
		chain = append(chain, caCert.Raw)
	}

	return tls.Certificate{
		Certificate: chain,
		PrivateKey:  privKey,
		Leaf:        cert,
	}, nil
}

// downloadCertificate 下载证书
func (ch *CertificateHandler) downloadCertificate() error {
	util.Info("正在从服务器请求证书...")

	certURL := ch.getServerConnectionURL("get_cert")

	dl := download.NewFileDownloaderWithOutput(certURL, 10000, 300000, ch.certFile, false)
	if err := dl.DownloadFile(); err != nil {
		return fmt.Errorf("下载证书失败")
	}

	if _, err := os.Stat(ch.certFile); os.IsNotExist(err) {
		return fmt.Errorf("证书文件下载失败")
	}

	return nil
}

// getServerConnectionURL 获取服务器连接 URL
func (ch *CertificateHandler) getServerConnectionURL(act string) string {
	settings := config.GetSettings()
	correctedTime := settings.GetServerTime()
	actkey := util.GetSHA1String(fmt.Sprintf("hentai@home-%s--%d-%d-%s",
		act, settings.GetClientID(), correctedTime, settings.GetClientKey()))

	return fmt.Sprintf("%s%s/%sclientbuild=%d&act=%s&add=&cid=%d&acttime=%d&actkey=%s",
		config.CLIENT_RPC_PROTOCOL, settings.GetRPCServerHost(), settings.GetRPCPath(),
		config.CLIENT_BUILD, act, settings.GetClientID(), correctedTime, actkey)
}

// IsCertExpiring 检查证书是否即将过期（24小时内）
func (ch *CertificateHandler) IsCertExpiring() bool {
	now := time.Now()
	return ch.certExpiry.Before(now.Add(24 * time.Hour))
}

// RefreshCertificate 刷新证书
func (ch *CertificateHandler) RefreshCertificate() error {
	util.Info("正在刷新证书...")

	if err := os.Remove(ch.certFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除旧证书失败: %w", err)
	}

	return ch.downloadCertificate()
}

// GetTLSCertificate 获取 TLS 证书配置
func (ch *CertificateHandler) GetTLSCertificate() (tls.Certificate, error) {
	cert, err := ch.loadPKCS12Certificate()
	if err != nil {
		return tls.Certificate{}, err
	}
	return cert, nil
}

// GetTLSConfig 获取 TLS 配置
func (ch *CertificateHandler) GetTLSConfig() (*tls.Config, error) {
	cert, err := ch.GetTLSCertificate()
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
