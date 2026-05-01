package config

// Config holds all runtime configuration for the server.
type Config struct {
	Host     string
	Port     int
	DataDir  string
	SSHHost  string
	SSHPort  int // 0 = random
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Host:    "0.0.0.0",
		Port:    8080,
		DataDir: "./data",
		SSHHost: "127.0.0.1",
		SSHPort: 0,
	}
}
