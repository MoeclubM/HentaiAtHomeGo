package server

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/javacompat"
	"github.com/qwq/hentaiathomego/internal/util"
)

func fileHotlinkKeystamp(fileID string, keystampTime int64, clientKey string) string {
	return util.GetSHA1String(fmt.Sprintf("%d-%s-%s-hotlinkthis", keystampTime, fileID, clientKey))[:10]
}

func TestFileRequestAuthGateMatchesJavaOracle(t *testing.T) {
	oracle, err := javacompat.Prepare()
	if err != nil {
		t.Skipf("java oracle unavailable: %v", err)
	}

	const (
		clientID  = 12345
		clientKey = "abcdefghijklmnopqrst"
		fileID    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-1-jpg"
	)
	serverTime := time.Now().Unix()
	configureServercmdTestSettings(t, "127.0.0.1", clientID, clientKey, serverTime)
	validStamp := fmt.Sprintf("%d-%s", serverTime, fileHotlinkKeystamp(fileID, serverTime, clientKey))

	tests := []struct {
		name    string
		request string
	}{
		{
			name:    "InvalidFileIDStillPrefers403OnBadKeystamp",
			request: fmt.Sprintf("GET /h/%s/keystamp=%s;fileindex=1;xres=org HTTP/1.1", "bad", fmt.Sprintf("%d-badbadbadb", serverTime)),
		},
		{
			name:    "InvalidKeystamp",
			request: fmt.Sprintf("GET /h/%s/keystamp=%s;fileindex=1;xres=org HTTP/1.1", fileID, fmt.Sprintf("%d-badbadbadb", serverTime)),
		},
		{
			name:    "MissingFileIndex",
			request: fmt.Sprintf("GET /h/%s/keystamp=%s;xres=org HTTP/1.1", fileID, validStamp),
		},
		{
			name:    "InvalidXRes",
			request: fmt.Sprintf("GET /h/%s/keystamp=%s;fileindex=1;xres=bad HTTP/1.1", fileID, validStamp),
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
				"status":        strconv.Itoa(response.GetResponseStatusCode()),
				"head":          strconv.FormatBool(response.IsRequestHeadOnly()),
				"servercmd":     strconv.FormatBool(response.IsServercmd()),
				"contentType":   processor.GetContentType(),
				"contentLength": strconv.Itoa(processor.GetContentLength()),
				"header":        processor.GetHeader(),
				"body":          readProcessorBody(t, response),
			}

			for _, key := range []string{"status", "head", "servercmd", "contentType", "contentLength", "header", "body"} {
				if actual[key] != expected[key] {
					t.Fatalf("mismatch for %s: got %q want %q", key, actual[key], expected[key])
				}
			}
		})
	}
}
