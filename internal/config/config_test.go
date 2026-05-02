package config

import "testing"

func TestDefault_HostIsLoopback(t *testing.T) {
	cfg := Default()
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("expected Host 127.0.0.1, got %q", cfg.Host)
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp/data"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	for _, port := range []int{-1, 0, 65536, 99999} {
		cfg := &Config{Host: "127.0.0.1", Port: port, DataDir: "/tmp/data"}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error for port %d", port)
		}
	}
}

func TestValidate_EmptyDataDir(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: 8080, DataDir: ""}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty DataDir")
	}
}
