package config

import "testing"

func TestInputQueryHandlerCLIPrefilledCredentials(t *testing.T) {
	t.Setenv("HATH_CLIENT_ID", "51839")
	t.Setenv("HATH_CLIENT_KEY", "AbCdEf123456GhIj7890")

	handler, err := NewInputQueryHandlerCLI()
	if err != nil {
		t.Fatalf("NewInputQueryHandlerCLI returned error: %v", err)
	}

	clientID, err := handler.QueryString("输入客户端 ID")
	if err != nil {
		t.Fatalf("QueryString for client id returned error: %v", err)
	}
	if clientID != "51839" {
		t.Fatalf("unexpected client id: got %q want %q", clientID, "51839")
	}

	clientKey, err := handler.QueryString("输入客户端密钥")
	if err != nil {
		t.Fatalf("QueryString for client key returned error: %v", err)
	}
	if clientKey != "AbCdEf123456GhIj7890" {
		t.Fatalf("unexpected client key: got %q want %q", clientKey, "AbCdEf123456GhIj7890")
	}
}

func TestInputQueryHandlerCLIUsesLegacyEnvNames(t *testing.T) {
	t.Setenv("HATHGO_CLIENT_ID", "60000")
	t.Setenv("HATHGO_CLIENT_KEY", "ZyXwVu987654TsRq3210")

	handler, err := NewInputQueryHandlerCLI()
	if err != nil {
		t.Fatalf("NewInputQueryHandlerCLI returned error: %v", err)
	}

	clientID, err := handler.QueryString("输入客户端 ID")
	if err != nil {
		t.Fatalf("QueryString for client id returned error: %v", err)
	}
	if clientID != "60000" {
		t.Fatalf("unexpected client id: got %q want %q", clientID, "60000")
	}

	clientKey, err := handler.QueryString("输入客户端密钥")
	if err != nil {
		t.Fatalf("QueryString for client key returned error: %v", err)
	}
	if clientKey != "ZyXwVu987654TsRq3210" {
		t.Fatalf("unexpected client key: got %q want %q", clientKey, "ZyXwVu987654TsRq3210")
	}
}
