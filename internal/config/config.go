package config

import (
	"fmt"
	"os"
	"runtime"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	S3       S3Config       `yaml:"s3"`
	Blast    BlastConfig    `yaml:"blast"`
	Database DatabaseConfig `yaml:"database"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type StorageConfig struct {
	Type           string `yaml:"type"`
	DataDir        string `yaml:"data_dir"`
	ResultTTLHours int    `yaml:"result_ttl_hours"`
}

type S3Config struct {
	Endpoint  string `yaml:"endpoint"`
	Bucket    string `yaml:"bucket"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
}

type BlastConfig struct {
	Path      string `yaml:"path"`
	MaxJobs   int    `yaml:"max_jobs"`
	CPUPerJob int    `yaml:"cpu_per_job"`
}

type DatabaseConfig struct {
	ConfigPath string `yaml:"config_path"`
}

type ResourceInfo struct {
	MaxJobs          int    `yaml:"max_jobs"`
	CPUPerJob        int    `yaml:"cpu_per_job"`
	ActualConcurrent int    `yaml:"actual_concurrent"`
	Degraded         bool   `yaml:"degraded"`
	DegradedReason   string `yaml:"degraded_reason,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Storage.Type == "" {
		c.Storage.Type = "local"
	}
	if c.Storage.DataDir == "" {
		c.Storage.DataDir = "./data"
	}
	if c.Storage.ResultTTLHours == 0 {
		c.Storage.ResultTTLHours = 24
	}
	if c.Blast.MaxJobs == 0 {
		c.Blast.MaxJobs = 20
	}
	if c.Blast.CPUPerJob == 0 {
		c.Blast.CPUPerJob = 2
	}
	if c.Database.ConfigPath == "" {
		c.Database.ConfigPath = "./databases.yaml"
	}
}

func (c *Config) validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Server.Port)
	}
	if c.Storage.Type != "local" && c.Storage.Type != "s3" {
		return fmt.Errorf("invalid storage type: %s (must be 'local' or 's3')", c.Storage.Type)
	}
	if c.Storage.Type == "s3" {
		if c.S3.Endpoint == "" {
			return fmt.Errorf("s3.endpoint is required when storage type is 's3'")
		}
		if c.S3.Bucket == "" {
			return fmt.Errorf("s3.bucket is required when storage type is 's3'")
		}
	}
	if c.Storage.ResultTTLHours < 1 {
		return fmt.Errorf("result_ttl_hours must be at least 1")
	}
	if c.Blast.MaxJobs < 1 {
		return fmt.Errorf("blast.max_jobs must be at least 1")
	}
	if c.Blast.CPUPerJob < 1 {
		return fmt.Errorf("blast.cpu_per_job must be at least 1")
	}
	return nil
}

func (c *Config) ComputeResources() ResourceInfo {
	cfg := c.Blast
	info := ResourceInfo{
		MaxJobs:   cfg.MaxJobs,
		CPUPerJob: cfg.CPUPerJob,
	}

	cpuConcurrent := runtime.NumCPU() / cfg.CPUPerJob
	if cpuConcurrent < 1 {
		cpuConcurrent = 1
	}

	memConcurrent := detectMemoryLimit()
	if memConcurrent < 1 {
		memConcurrent = 1
	}

	actual := cfg.MaxJobs
	if cpuConcurrent < actual {
		actual = cpuConcurrent
	}
	if memConcurrent < actual {
		actual = memConcurrent
	}

	info.ActualConcurrent = actual

	if actual < 5 {
		info.Degraded = true
		if actual == cpuConcurrent && cpuConcurrent < 5 {
			info.DegradedReason = fmt.Sprintf("insufficient CPU: %d cores available, need %d per job (effective limit: %d)", runtime.NumCPU(), cfg.CPUPerJob, cpuConcurrent)
		} else if actual == memConcurrent && memConcurrent < 5 {
			info.DegradedReason = fmt.Sprintf("insufficient memory: effective limit %d jobs", memConcurrent)
		} else {
			info.DegradedReason = fmt.Sprintf("max_jobs capped at %d", actual)
		}
	}

	return info
}

func detectMemoryLimit() int {
	if runtime.GOOS == "linux" {
		return detectMemoryLimitLinux()
	}
	return detectMemoryLimitDefault()
}
