package server

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/javacompat"
	"github.com/qwq/hentaiathomego/internal/server/processors"
)

func speedtestKey(testSize int, testTime int64, clientID int, clientKey string) string {
	payload := fmt.Sprintf("hentai@home-speedtest-%d-%d-%d-%s", testSize, testTime, clientID, clientKey)
	hash := sha1.Sum([]byte(payload))
	return hex.EncodeToString(hash[:])
}

func TestSpeedtestPathMatchesJavaOracle(t *testing.T) {
	oracle, err := javacompat.Prepare()
	if err != nil {
		t.Skipf("java oracle unavailable: %v", err)
	}

	const (
		clientID  = 12345
		clientKey = "abcdefghijklmnopqrst"
		testSize  = 64
	)
	serverTime := time.Now().Unix()
	configureServercmdTestSettings(t, "127.0.0.1", clientID, clientKey, serverTime)

	tests := []struct {
		name        string
		request     string
		compareBody bool
	}{
		{
			name:        "Valid",
			request:     fmt.Sprintf("GET /t/%d/%d/%s HTTP/1.1", testSize, serverTime, speedtestKey(testSize, serverTime, clientID, clientKey)),
			compareBody: false,
		},
		{
			name:        "InvalidKey",
			request:     fmt.Sprintf("GET /t/%d/%d/%s HTTP/1.1", testSize, serverTime, "badkey"),
			compareBody: true,
		},
		{
			name:        "Expired",
			request:     fmt.Sprintf("GET /t/%d/%d/%s HTTP/1.1", testSize, serverTime-301, speedtestKey(testSize, serverTime-301, clientID, clientKey)),
			compareBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, err := oracle.Run(
				"parse-request-config",
				tt.request,
				"false",
				strconv.Itoa(clientID),
				clientKey,
				strconv.FormatInt(serverTime, 10),
			)
			if err != nil {
				t.Fatalf("running java oracle failed: %v", err)
			}

			response := NewResponse(nil)
			response.ParseRequest(tt.request, false)
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

			actual["processorClass"] = map[string]string{
				"*processors.HTTPResponseProcessorText":      "HTTPResponseProcessorText",
				"*processors.HTTPResponseProcessorSpeedtest": "HTTPResponseProcessorSpeedtest",
			}[actual["processorClass"]]

			if _, ok := processor.(*processors.HTTPResponseProcessorText); ok && tt.compareBody {
				actual["body"] = readProcessorBody(t, response)
			}

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
