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
	PUBG    PUBGConfig    `json:"pubg"`
	MongoDB MongoDBConfig `json:"mongodb"`
	Logging LoggingConfig `json:"logging"`
	S3      S3Config      `json:"s3"`
	CORS    CORSConfig    `json:"cors"` // Add this new field
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
	RequestDelay      int               `json:"requestDelay"`
	MaxRetries        int               `json:"maxRetries"`
	RequestsPerMinute int               `json:"requestsPerMinute"`
	OldEnoughCap      int               `json:"oldEnoughCap"`
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

// S3Config contains S3 bucket connection details
type S3Config struct {
	AccessKeyID     string         `json:"accessKeyId"`
	SecretAccessKey string         `json:"secretAccessKey"`
	Region          string         `json:"region"`
	Bucket          string         `json:"bucket"`
	Endpoint        string         `json:"endpoint"`
	ForcePathStyle  bool           `json:"forcePathStyle"`
	UploadOptions   map[string]int `json:"uploadOptions"`
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
