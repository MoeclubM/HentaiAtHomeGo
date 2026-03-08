package network

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
)

func TestGetServerConnectionURLWithAddUsesRPCHost(t *testing.T) {
	settings := config.GetSettings()
	settings.ParseAndUpdateSettings([]string{
		"rpc_server_ip=1.2.3.4",
		"rpc_server_port=4567",
		"rpc_path=15/rpc?",
		"host=https://public.example",
		"server_time=" + time.Now().UTC().Format("1136239445"),
	})

	clientLoginURL := GetServerConnectionURLWithAdd(ACT_CLIENT_LOGIN, "")
	if !strings.HasPrefix(clientLoginURL, config.CLIENT_RPC_PROTOCOL+"1.2.3.4:4567/15/rpc?clientbuild=") {
		t.Fatalf("client_login URL uses unexpected prefix: %s", clientLoginURL)
	}
	if strings.Contains(clientLoginURL, "public.example") {
		t.Fatalf("client_login URL should not use client host: %s", clientLoginURL)
	}

	serverStatURL := GetServerConnectionURL(ACT_SERVER_STAT)
	if !strings.HasPrefix(serverStatURL, config.CLIENT_RPC_PROTOCOL+"1.2.3.4:4567/15/rpc?clientbuild=") {
		t.Fatalf("server_stat URL uses unexpected prefix: %s", serverStatURL)
	}
}

func TestGetServerResponseWithURLDropsTrailingEmptyLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := "OK\nline1\n\n"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	response := GetServerResponseWithURL(server.URL, nil)
	if response.GetResponseStatus() != RESPONSE_STATUS_OK {
		t.Fatalf("unexpected response status: %d", response.GetResponseStatus())
	}

	text := response.GetResponseText()
	if len(text) != 1 || text[0] != "line1" {
		t.Fatalf("unexpected response lines: %#v", text)
	}
}

func TestGetServerResponseWithURLSendsJavaUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "Hentai@Home "+config.CLIENT_VERSION {
			t.Fatalf("unexpected User-Agent: %q", got)
		}

		body := "OK\n"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	response := GetServerResponseWithURL(server.URL, nil)
	if response.GetResponseStatus() != RESPONSE_STATUS_OK {
		t.Fatalf("unexpected response status: %d", response.GetResponseStatus())
	}
}

func TestGetServerResponseWithURLUsesHostnameOnlyOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("OK\nignored\n"))
	}))
	defer server.Close()

	response := GetServerResponseWithURL(server.URL, nil)
	if response.GetResponseStatus() != RESPONSE_STATUS_NULL {
		t.Fatalf("unexpected response status: %d", response.GetResponseStatus())
	}

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parsing test server URL failed: %v", err)
	}
	if response.GetFailHost() != parsedURL.Hostname() {
		t.Fatalf("unexpected fail host: got %q want %q", response.GetFailHost(), parsedURL.Hostname())
	}
}

func TestGetServerResponseWithURLRejectsOversizedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := strings.Repeat("a", 10485761)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	response := GetServerResponseWithURL(server.URL, nil)
	if response.GetResponseStatus() != RESPONSE_STATUS_NULL {
		t.Fatalf("expected oversized payload to be rejected, got status %d", response.GetResponseStatus())
	}
}

func TestGetDownloaderFetchURLRejectsInvalidURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := "OK\n%bad%\n"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parsing test server URL failed: %v", err)
	}

	settings := config.GetSettings()
	settings.ParseAndUpdateSettings([]string{
		"rpc_server_ip=" + parsedURL.Hostname(),
		"rpc_server_port=" + parsedURL.Port(),
		"rpc_path=?",
		"server_time=" + time.Now().UTC().Format("1136239445"),
	})
	settings.ClearRPCServerFailure()

	handler := NewServerHandler(nil)
	if fetchURL := handler.GetDownloaderFetchURL(1, 2, 3, "x", 4); fetchURL != "" {
		t.Fatalf("expected invalid URL to be rejected, got %q", fetchURL)
	}
}
