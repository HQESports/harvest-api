package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the entire application configuration
type Config struct {
	Env      string         `json:"env"`
	Port     int            `json:"port"`
	AppName  string         `json:"app_name"`
	PUBG     PUBGConfig     `json:"pubg"`
	MongoDB  MongoDBConfig  `json:"mongodb"`
	Redis    RedisConfig    `json:"redis"`
	Logging  LoggingConfig  `json:"logging"`
	CORS     CORSConfig     `json:"cors"`
	RabbitMQ RabbitMQConfig `json:"rabbitmq"` // Add this new field
	Jobs     JobsConfig     `json:"jobs"`     // Add this new field
	AWS      AWSConfig      `json:"aws"`      // Add this new field
}

// AWSConfig contains AWS-related configurations
type AWSConfig struct {
	S3     S3Config `json:"s3"`
	Region string   `json:"region"`
}

// S3Config contains S3-specific configurations
type S3Config struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Bucket          string `json:"bucket"`
}

// RedisConfig contains Redis connection details
type RedisConfig struct {
	Address  string `json:"address"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	Prefix   string `json:"prefix"`
}

// CORSConfig contains Cross-Origin Resource Sharing settings
type CORSConfig struct {
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age,omitempty"`
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
	Cache             bool              `json:"cache"`
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

// RabbitMQConfig contains RabbitMQ connection and queue settings
type RabbitMQConfig struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	VHost         string `json:"vhost"`
	ExchangeName  string `json:"exchange_name"`
	QueueName     string `json:"queue_name"`
	RoutingKey    string `json:"routing_key"`
	PrefetchCount int    `json:"prefetch_count"`
	MaxRetries    int    `json:"max_retries"`
}

// JobsConfig contains job processing settings
type JobsConfig struct {
	WorkerCount      int             `json:"worker_count"`
	DefaultBatchSize int             `json:"default_batch_size"`
	JobTypes         []JobTypeConfig `json:"job_types"`
}

// JobTypeConfig contains configuration for a specific job type
type JobTypeConfig struct {
	Type      string `json:"type"`
	BatchSize int    `json:"batch_size"`
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
