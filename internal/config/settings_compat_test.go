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

func TestParseArgsOnlyAppliesLocalDirectorySettings(t *testing.T) {
	settings := GetSettings()

	settings.ParseAndUpdateSettings([]string{
		"host=remote.example.com",
		"port=24444",
		"cache_dir=cache-remote",
	})

	settings.ParseArgs([]string{
		"--host=local.example.com",
		"--port=12345",
		"--cache-dir=cache-local",
		"--data-dir=data-local",
	})

	if got := settings.GetClientHost(); got != "remote.example.com" {
		t.Fatalf("local args should not override remote host: got %q", got)
	}
	if got := settings.GetClientPort(); got != 24444 {
		t.Fatalf("local args should not override remote port: got %d", got)
	}
	if got := settings.GetCacheDir(); got != "cache-local" {
		t.Fatalf("local cache dir arg not applied: got %q", got)
	}
	if got := settings.GetDataDir(); got != "data-local" {
		t.Fatalf("local data dir arg not applied: got %q", got)
	}
	if got := settings.GetLogDir(); got == "" {
		t.Fatal("log dir should remain initialized")
	}

	settings.ParseAndUpdateSettings([]string{
		"host=",
		"port=0",
		"cache_dir=cache",
		"data_dir=data",
		"log_dir=log",
		"temp_dir=tmp",
		"download_dir=download",
	})
}
