package server

import (
	"net"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/config"
)

func readProcessorBody(t *testing.T, response *Response) string {
	t.Helper()
	processor := response.GetResponseProcessor()
	var builder strings.Builder
	for builder.Len() < processor.GetContentLength() {
		chunk, err := processor.GetPreparedTCPBuffer()
		if err != nil {
			t.Fatalf("reading processor body failed: %v", err)
		}
		if len(chunk) == 0 {
			break
		}
		builder.Write(chunk)
	}
	return builder.String()
}

func TestDefaultErrorBodyMatchesJava(t *testing.T) {
	response := NewResponse(nil)
	response.ParseRequest("POST / HTTP/1.1", false)

	body := readProcessorBody(t, response)
	if body != "An error has occurred. (405)" {
		t.Fatalf("unexpected default error body: %q", body)
	}
}

func TestRobotsTxtContentTypeMatchesJava(t *testing.T) {
	response := NewResponse(nil)
	response.ParseRequest("GET /robots.txt HTTP/1.1", false)

	processor := response.GetResponseProcessor()
	if processor.GetContentType() != "text/plain; charset=ISO-8859-1" {
		t.Fatalf("unexpected robots.txt content type: %s", processor.GetContentType())
	}

	body := readProcessorBody(t, response)
	if body != "User-agent: *\nDisallow: /" {
		t.Fatalf("unexpected robots.txt body: %q", body)
	}
}

func TestDoTimeoutCheckUsesThirtySecondsBeforeResponse(t *testing.T) {
	now := time.Now()
	session := &Session{
		sessionStartTime: now.Add(-31 * time.Second),
		lastPacketSend:   now,
	}
	if !session.DoTimeoutCheck(now) {
		t.Fatalf("expected timeout for session older than 30s before response exists")
	}

	session = &Session{
		sessionStartTime: now.Add(-29 * time.Second),
		lastPacketSend:   now,
	}
	if session.DoTimeoutCheck(now) {
		t.Fatalf("did not expect timeout for session newer than 30s before response exists")
	}
}

func TestIsLocalNetworkStripsIPv6MappedPrefixFromClientHost(t *testing.T) {
	settings := config.GetSettings()
	settings.ParseAndUpdateSettings([]string{"host=::ffff:127.0.0.1"})

	server := &Server{
		localNetworkPattern: regexp.MustCompile(`^((localhost)|(127\.)|(10\.)|(192\.168\.)|(172\.((1[6-9])|(2[0-9])|(3[0-1]))\.)|(169\.254\.)|(::1)|(0:0:0:0:0:0:0:1)|(fc)|(fd)).*$`),
	}

	if !server.isLocalNetwork(net.ParseIP("127.0.0.1").String()) {
		t.Fatalf("expected IPv6-mapped client host to match IPv4 loopback")
	}
}
