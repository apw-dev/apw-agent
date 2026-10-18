package conf

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ErrInvalidPort reports a port outside 1-65535.
var ErrInvalidPort = errors.New("invalid port")

// Config is the server configuration.
type Config struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Load reads a JSON configuration file and applies defaults.
func Load(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port == 0 {
		c.Port = 8080
	}
	if c.Port < 1 || c.Port > 65535 {
		return c, fmt.Errorf("port %d: %w", c.Port, ErrInvalidPort)
	}
	return c, nil
}
