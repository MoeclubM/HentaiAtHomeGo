package server

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/qwq/hentaiathomego/internal/javacompat"
	"github.com/qwq/hentaiathomego/internal/network"
)

func TestMalformedServercmdActtimeAbortsConnectionLikeJava(t *testing.T) {
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

	request := fmt.Sprintf("GET /servercmd/still_alive//badtime/%s HTTP/1.1", servercmdKey("still_alive", "", serverTime, clientID, clientKey))
	if _, err := oracle.Run("servercmd", request, rpcIP, strconv.Itoa(clientID), clientKey, strconv.FormatInt(serverTime, 10)); err == nil {
		t.Fatal("expected java oracle to abort on malformed servercmd acttime")
	}

	dummyNetClient := &dummyNetworkClient{}
	dummySrvClient := &dummyServerClient{serverHandler: network.NewServerHandler(dummyNetClient)}
	server := &Server{client: dummySrvClient}
	session, cleanup := newLoopbackTLSSession(t, server)
	defer cleanup()

	response := NewResponse(session)
	response.ParseRequest(request, false)
	if !response.ShouldAbortConnection() {
		t.Fatal("expected malformed servercmd acttime to abort connection")
	}
}

func TestMalformedSpeedtestNumbersAbortConnectionLikeJava(t *testing.T) {
	oracle, err := javacompat.Prepare()
	if err != nil {
		t.Skipf("java oracle unavailable: %v", err)
	}

	request := "GET /t/notanint/123/key HTTP/1.1"
	if _, err := oracle.Run("parse-request", request, "false"); err == nil {
		t.Fatal("expected java oracle to abort on malformed speedtest size")
	}

	response := NewResponse(nil)
	response.ParseRequest(request, false)
	if !response.ShouldAbortConnection() {
		t.Fatal("expected malformed speedtest size to abort connection")
	}

	request = "GET /t/1/notanint/key HTTP/1.1"
	if _, err := oracle.Run("parse-request", request, "false"); err == nil {
		t.Fatal("expected java oracle to abort on malformed speedtest time")
	}

	response = NewResponse(nil)
	response.ParseRequest(request, false)
	if !response.ShouldAbortConnection() {
		t.Fatal("expected malformed speedtest time to abort connection")
	}
}

func TestMalformedThreadedProxyProtocolAbortsConnectionLikeJava(t *testing.T) {
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

	additional := "hostname=example.com;protocol=%bad%;port=80;testsize=1;testcount=1;testtime=1;testkey=x"
	request := fmt.Sprintf("GET /servercmd/threaded_proxy_test/%s/%d/%s HTTP/1.1", additional, serverTime, servercmdKey("threaded_proxy_test", additional, serverTime, clientID, clientKey))
	if _, err := oracle.Run("servercmd", request, rpcIP, strconv.Itoa(clientID), clientKey, strconv.FormatInt(serverTime, 10)); err == nil {
		t.Fatal("expected java oracle to abort on malformed threaded_proxy_test protocol")
	}

	dummyNetClient := &dummyNetworkClient{}
	dummySrvClient := &dummyServerClient{serverHandler: network.NewServerHandler(dummyNetClient)}
	server := &Server{client: dummySrvClient}
	session, cleanup := newLoopbackTLSSession(t, server)
	defer cleanup()

	response := NewResponse(session)
	response.ParseRequest(request, false)
	if !response.ShouldAbortConnection() {
		t.Fatal("expected malformed threaded_proxy_test protocol to abort connection")
	}
}
