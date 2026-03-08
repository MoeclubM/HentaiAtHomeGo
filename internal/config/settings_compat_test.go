package config

import (
	"net"
	"strings"
	"testing"
)

func TestGetRPCServerHostSkipsLastFailedServer(t *testing.T) {
	settings := GetSettings()

	settings.rpcServerLock.Lock()
	oldServers := settings.rpcServers
	oldPort := settings.rpcServerPort
	oldCurrent := settings.rpcServerCurrent
	oldLastFailed := settings.rpcServerLastFailed
	settings.rpcServers = []net.IP{net.ParseIP("203.0.113.10"), net.ParseIP("203.0.113.11")}
	settings.rpcServerPort = 4567
	settings.rpcServerCurrent = ""
	settings.rpcServerLastFailed = "203.0.113.10"
	settings.rpcServerLock.Unlock()

	defer func() {
		settings.rpcServerLock.Lock()
		settings.rpcServers = oldServers
		settings.rpcServerPort = oldPort
		settings.rpcServerCurrent = oldCurrent
		settings.rpcServerLastFailed = oldLastFailed
		settings.rpcServerLock.Unlock()
	}()

	host := settings.GetRPCServerHost()
	if strings.HasPrefix(host, "203.0.113.10") {
		t.Fatalf("GetRPCServerHost selected last failed server: %s", host)
	}
	if host != "203.0.113.11:4567" {
		t.Fatalf("unexpected selected RPC server: %s", host)
	}
}
