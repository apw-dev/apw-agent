package conf

import (
	"encoding/json"
	"os"
)

// Config is the server configuration.
type Config struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Load reads a JSON configuration file.
func Load(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(data, &c)
	return c, err
}
