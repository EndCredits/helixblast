package config

import (
	"fmt"
	"os"
	"runtime"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Jobs     JobsConfig     `yaml:"jobs"`
	Blast    BlastConfig    `yaml:"blast"`
	Database DatabaseConfig `yaml:"database"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

// JobsConfig governs the in-memory job registry. Results are never persisted
// server-side (IndexedDB-first architecture): a terminal job lives in memory
// for ResultTTLHours, then the registry pruner drops it and its ID resolves
// to 404.
type JobsConfig struct {
	ResultTTLHours int `yaml:"result_ttl_hours"`
}

type BlastConfig struct {
	Path      string `yaml:"path"`
	MaxJobs   int    `yaml:"max_jobs"`
	CPUPerJob int    `yaml:"cpu_per_job"`
}

type DatabaseConfig struct {
	ConfigPath string `yaml:"config_path"`
	WorkerURL  string `yaml:"worker_url"`
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
	if c.Jobs.ResultTTLHours == 0 {
		c.Jobs.ResultTTLHours = 24
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
	if c.Jobs.ResultTTLHours < 1 {
		return fmt.Errorf("jobs.result_ttl_hours must be at least 1")
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
