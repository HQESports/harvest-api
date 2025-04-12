package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the entire application configuration
type Config struct {
	Env     string        `json:"env"`
	Port    int           `json:"port"`
	AppName string        `json:"app_name"`
	PUBG    PUBGConfig    `json:"pubg"`
	MongoDB MongoDBConfig `json:"mongodb"`
	Redis   RedisConfig   `json:"redis"`
	Logging LoggingConfig `json:"logging"`
	CORS    CORSConfig    `json:"cors"` // Add this new field
}

type RedisConfig struct {
	Address  string
	Password string
	DB       int
	Prefix   string
}

// CORSConfig contains Cross-Origin Resource Sharing settings
type CORSConfig struct {
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age,omitempty"` // Optional, seconds that preflight requests can be cached
}

// PUBGConfig contains PUBG API-related configurations
type PUBGConfig struct {
	APIKey            string            `json:"api_key"`
	BaseURL           string            `json:"base_url"`
	Shards            map[string]string `json:"shards"`
	RequestDelay      int               `json:"request_delay"`
	MaxRetries        int               `json:"max_retries"`
	RequestsPerMinute int               `json:"requests_per_minute"`
	OldEnoughMin      int               `json:"old_enough_min"`
	Cache             bool              `json:"cahce"`
	DefaultCacheTTL   int               `json:"default_cache_ttl"`
}

// MongoDBConfig contains MongoDB connection details
type MongoDBConfig struct {
	URI      string                 `json:"uri"`
	Username string                 `json:"username"`
	Password string                 `json:"password"`
	DB       string                 `json:"db"`
	Options  map[string]interface{} `json:"options"`
}

// LoggingConfig contains logging-related configurations
type LoggingConfig struct {
	Level     string `json:"level"`
	Format    string `json:"format"`
	Directory string `json:"directory"`
}

// LoadConfig reads configuration from the specified file path
func LoadConfig(filePath string) (*Config, error) {
	// Read the configuration file
	configData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Create a new Config struct
	var config Config

	// Unmarshal the JSON data into the Config struct
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	return &config, nil
}
