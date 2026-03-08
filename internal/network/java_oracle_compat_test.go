package network

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
	"github.com/qwq/hentaiathomego/internal/javacompat"
)

func TestServerResponseMatchesJavaOracle(t *testing.T) {
	oracle, err := javacompat.Prepare()
	if err != nil {
		t.Skipf("java oracle unavailable: %v", err)
	}

	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "OKTrailingEmptyLines",
			handler: func(w http.ResponseWriter, r *http.Request) {
				body := "OK\nline1\n\n"
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
				_, _ = w.Write([]byte(body))
			},
		},
		{
			name: "TemporarilyUnavailable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				body := "TEMPORARILY_UNAVAILABLE\n"
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
				_, _ = w.Write([]byte(body))
			},
		},
		{
			name: "ServerErrorBecomesNoResponse",
			handler: func(w http.ResponseWriter, r *http.Request) {
				body := "FAIL\n"
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(body))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			expected, err := oracle.Run("server-response", server.URL)
			if err != nil {
				t.Fatalf("running java oracle failed: %v", err)
			}

			response := GetServerResponseWithURL(server.URL, nil)
			actual := map[string]string{
				"status":    strconv.Itoa(response.GetResponseStatus()),
				"failCode":  response.GetFailCode(),
				"failHost":  response.GetFailHost(),
				"textCount": strconv.Itoa(len(response.GetResponseText())),
				"text":      "",
			}
			if text := response.GetResponseText(); len(text) > 0 {
				actual["text"] = text[0]
				for i := 1; i < len(text); i++ {
					actual["text"] += "\n" + text[i]
				}
			}

			for _, key := range []string{"status", "failCode", "failHost", "textCount", "text"} {
				if actual[key] != expected[key] {
					t.Fatalf("mismatch for %s: got %q want %q", key, actual[key], expected[key])
				}
			}
		})
	}
}

func TestRPCURLGenerationMatchesJavaOracle(t *testing.T) {
	oracle, err := javacompat.Prepare()
	if err != nil {
		t.Skipf("java oracle unavailable: %v", err)
	}

	settings := config.GetSettings()

	tests := []struct {
		name string
		act  string
		add  string
	}{
		{name: "ClientLogin", act: ACT_CLIENT_LOGIN, add: ""},
		{name: "StaticRangeFetch", act: ACT_STATIC_RANGE_FETCH, add: "123;org;abcdef0123456789abcdef12"},
		{name: "ServerStat", act: ACT_SERVER_STAT, add: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lastActual string
			var lastExpected string
			for attempt := 0; attempt < 3; attempt++ {
				serverTime := strconv.FormatInt(time.Now().Unix(), 10)
				settings.ParseAndUpdateSettings([]string{
					"rpc_server_ip=1.2.3.4",
					"rpc_server_port=4567",
					"rpc_path=15/rpc?",
					"server_time=" + serverTime,
				})
				settings.ClearRPCServerFailure()

				expected, err := oracle.Run(
					"url-query",
					tt.act,
					tt.add,
					"0",
					"",
					serverTime,
					"1.2.3.4",
					"4567",
					"15/rpc?",
				)
				if err != nil {
					t.Fatalf("running java oracle failed: %v", err)
				}

				actualURL := GetServerConnectionURLWithAdd(tt.act, tt.add)
				if tt.act == ACT_SERVER_STAT {
					actualURL = GetServerConnectionURL(tt.act)
				}

				lastActual = actualURL
				lastExpected = expected["url"]
				if actualURL == expected["url"] {
					return
				}
			}

			t.Fatalf("url mismatch: got %q want %q", lastActual, lastExpected)
		})
	}
}
