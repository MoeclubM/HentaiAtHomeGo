package download

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qwq/hentaiathomego/pkg/hvfile"
)

func TestOpenProxySourceReturns500OnConnectFailure(t *testing.T) {
	pfd := NewProxyFileDownloader(nil, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-1-jpg", []string{"http://127.0.0.1:1/test"})
	hv, err := hvfile.GetHVFileFromFileid("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-1-jpg")
	if err != nil {
		t.Fatalf("unexpected hvfile parse error: %v", err)
	}

	resp, cancel, status := pfd.openProxySource("http://127.0.0.1:1/test", hv)
	if cancel != nil {
		cancel()
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	if status != 500 {
		t.Fatalf("unexpected status: got %d want 500", status)
	}
}

func TestInitializePreserves502OverLater500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := "ab"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	pfd := NewProxyFileDownloader(nil, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-1-jpg", []string{server.URL, "http://127.0.0.1:1/test"})
	if status := pfd.Initialize(); status != 502 {
		t.Fatalf("unexpected status: got %d want 502", status)
	}

	hv, err := hvfile.GetHVFileFromFileid("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-1-jpg")
	if err != nil {
		t.Fatalf("unexpected hvfile parse error: %v", err)
	}
	_, _, _ = pfd.openProxySource(server.URL, hv)
}
