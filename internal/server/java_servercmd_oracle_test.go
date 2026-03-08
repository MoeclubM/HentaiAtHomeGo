package server

import (
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/cache"
	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/javacompat"
	"github.com/qwq/hentaiathomego/internal/network"
	"github.com/qwq/hentaiathomego/internal/util"
)

type dummyServerClient struct {
	serverHandler *network.ServerHandler
}

func (d *dummyServerClient) NotifyOverload()                          {}
func (d *dummyServerClient) IsShuttingDown() bool                     { return false }
func (d *dummyServerClient) GetCacheHandler() *cache.CacheHandler     { return nil }
func (d *dummyServerClient) GetServerHandler() *network.ServerHandler { return d.serverHandler }
func (d *dummyServerClient) StartDownloader()                         {}
func (d *dummyServerClient) SetCertRefresh()                          {}
func (d *dummyServerClient) DieWithError(error string)                {}

type dummyNetworkClient struct{}

func (d *dummyNetworkClient) GetInputQueryHandler() util.InputQueryHandler { return nil }
func (d *dummyNetworkClient) DieWithError(error string)                    {}
func (d *dummyNetworkClient) PromptForIDAndKey()                           {}

func configureServercmdTestSettings(t *testing.T, rpcIP string, clientID int, clientKey string, serverTime int64) {
	t.Helper()
	settings := config.GetSettings()
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, config.CLIENT_LOGIN_FILENAME), []byte(fmt.Sprintf("%d-%s", clientID, clientKey)), 0644); err != nil {
		t.Fatalf("writing client_login failed: %v", err)
	}
	settings.ParseAndUpdateSettings([]string{
		"data_dir=" + tempDir,
		"rpc_server_ip=" + rpcIP,
		"server_time=" + strconv.FormatInt(serverTime, 10),
	})
	if !settings.LoadClientLoginFromFile() {
		t.Fatalf("loading client_login failed")
	}
}

func newLoopbackTLSSession(t *testing.T, server *Server) (*Session, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
		close(accepted)
	}()

	rawClientConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		listener.Close()
		t.Fatalf("dial failed: %v", err)
	}

	serverConn := <-accepted
	clientConn := tls.Client(rawClientConn, &tls.Config{InsecureSkipVerify: true})
	session := NewSession(clientConn, 1, false, server)

	cleanup := func() {
		_ = clientConn.Close()
		if serverConn != nil {
			_ = serverConn.Close()
		}
		_ = listener.Close()
	}

	return session, cleanup
}

func servercmdKey(command, additional string, commandTime int64, clientID int, clientKey string) string {
	payload := fmt.Sprintf("hentai@home-servercmd-%s-%s-%d-%d-%s", command, additional, clientID, commandTime, clientKey)
	hash := sha1.Sum([]byte(payload))
	return hex.EncodeToString(hash[:])
}

func TestServercmdMatchesJavaOracle(t *testing.T) {
	oracle, err := javacompat.Prepare()
	if err != nil {
		t.Skipf("java oracle unavailable: %v", err)
	}

	const (
		rpcIP     = "127.0.0.1"
		clientID  = 12345
		clientKey = "abcdefghijklmnopqrst"
	)
	serverTime := time.Now().Unix()
	configureServercmdTestSettings(t, rpcIP, clientID, clientKey, serverTime)

	dummyNetClient := &dummyNetworkClient{}
	dummySrvClient := &dummyServerClient{serverHandler: network.NewServerHandler(dummyNetClient)}
	server := &Server{client: dummySrvClient}
	session, cleanup := newLoopbackTLSSession(t, server)
	defer cleanup()

	tests := []struct {
		name          string
		command       string
		additional    string
		compareBody   bool
		compareLength bool
	}{
		{name: "StillAlive", command: "still_alive", additional: "", compareBody: true, compareLength: true},
		{name: "SpeedTestInvalid", command: "speed_test", additional: "testsize=abc", compareBody: true, compareLength: true},
		{name: "SpeedTestValid", command: "speed_test", additional: "testsize=64", compareBody: false, compareLength: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := servercmdKey(tt.command, tt.additional, serverTime, clientID, clientKey)
			request := fmt.Sprintf("GET /servercmd/%s/%s/%d/%s HTTP/1.1", tt.command, tt.additional, serverTime, key)

			expected, err := oracle.Run("servercmd", request, rpcIP, strconv.Itoa(clientID), clientKey, strconv.FormatInt(serverTime, 10))
			if err != nil {
				t.Fatalf("running java oracle failed: %v", err)
			}

			response := NewResponse(session)
			response.ParseRequest(request, false)
			processor := response.GetResponseProcessor()
			actual := map[string]string{
				"status":         strconv.Itoa(response.GetResponseStatusCode()),
				"head":           strconv.FormatBool(response.IsRequestHeadOnly()),
				"servercmd":      strconv.FormatBool(response.IsServercmd()),
				"contentType":    processor.GetContentType(),
				"contentLength":  strconv.Itoa(processor.GetContentLength()),
				"header":         processor.GetHeader(),
				"processorClass": fmt.Sprintf("%T", processor),
				"body":           "",
			}

			if tt.compareBody {
				actual["body"] = readProcessorBody(t, response)
			}
			if !tt.compareLength {
				actual["contentLength"] = expected["contentLength"]
			}

			actual["processorClass"] = map[string]string{
				"*processors.HTTPResponseProcessorText":      "HTTPResponseProcessorText",
				"*processors.HTTPResponseProcessorSpeedtest": "HTTPResponseProcessorSpeedtest",
			}[actual["processorClass"]]

			for _, key := range []string{"status", "head", "servercmd", "contentType", "contentLength", "header", "processorClass"} {
				if actual[key] != expected[key] {
					t.Fatalf("mismatch for %s: got %q want %q", key, actual[key], expected[key])
				}
			}
			if tt.compareBody && actual["body"] != expected["body"] {
				t.Fatalf("mismatch for body: got %q want %q", actual["body"], expected["body"])
			}
		})
	}
}
