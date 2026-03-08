package server

import (
	"strconv"
	"testing"

	"github.com/qwq/hentaiathomego/internal/javacompat"
)

func TestRequestParsingMatchesJavaOracle(t *testing.T) {
	oracle, err := javacompat.Prepare()
	if err != nil {
		t.Skipf("java oracle unavailable: %v", err)
	}

	tests := []struct {
		name    string
		request string
		local   bool
	}{
		{name: "MethodNotAllowed", request: "POST / HTTP/1.1"},
		{name: "Robots", request: "GET /robots.txt HTTP/1.1"},
		{name: "HeadRobots", request: "HEAD /robots.txt HTTP/1.1"},
		{name: "AbsoluteHttpURI", request: "GET http://example.com/robots.txt HTTP/1.1"},
		{name: "AbsoluteHttpsURINotNormalized", request: "GET https://example.com/robots.txt HTTP/1.1"},
		{name: "FaviconRedirect", request: "GET /favicon.ico HTTP/1.1"},
		{name: "BadRelativePath", request: "GET foo HTTP/1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, err := oracle.Run("parse-request", tt.request, strconv.FormatBool(tt.local))
			if err != nil {
				t.Fatalf("running java oracle failed: %v", err)
			}

			response := NewResponse(nil)
			response.ParseRequest(tt.request, tt.local)
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
