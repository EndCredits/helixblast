package config

import (
	"os"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	tmp := writeTempYAML(t, `
server:
  port: 9090
storage:
  type: local
  result_ttl_hours: 48
blast:
  max_jobs: 10
  cpu_per_job: 4
database:
  config_path: ./databases.yaml
`)
	defer os.Remove(tmp)

	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Storage.Type != "local" {
		t.Errorf("expected local storage, got %s", cfg.Storage.Type)
	}
	if cfg.Storage.ResultTTLHours != 48 {
		t.Errorf("expected TTL 48, got %d", cfg.Storage.ResultTTLHours)
	}
}

func TestLoadDefaults(t *testing.T) {
	tmp := writeTempYAML(t, `{}`)
	defer os.Remove(tmp)

	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Storage.Type != "local" {
		t.Errorf("expected default local storage, got %s", cfg.Storage.Type)
	}
	if cfg.Storage.ResultTTLHours != 24 {
		t.Errorf("expected default TTL 24, got %d", cfg.Storage.ResultTTLHours)
	}
	if cfg.Blast.MaxJobs != 20 {
		t.Errorf("expected default max_jobs 20, got %d", cfg.Blast.MaxJobs)
	}
	if cfg.Blast.CPUPerJob != 2 {
		t.Errorf("expected default cpu_per_job 2, got %d", cfg.Blast.CPUPerJob)
	}
}

func TestLoadInvalidStorage(t *testing.T) {
	tmp := writeTempYAML(t, `
storage:
  type: unknown
`)
	defer os.Remove(tmp)

	_, err := Load(tmp)
	if err == nil {
		t.Error("expected error for invalid storage type")
	}
}

func TestComputeResources(t *testing.T) {
	tmp := writeTempYAML(t, `
blast:
  max_jobs: 20
  cpu_per_job: 2
`)
	defer os.Remove(tmp)

	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	resources := cfg.ComputeResources()
	if resources.MaxJobs != 20 {
		t.Errorf("expected max_jobs 20, got %d", resources.MaxJobs)
	}
	if resources.ActualConcurrent < 1 {
		t.Errorf("actual_concurrent should be at least 1, got %d", resources.ActualConcurrent)
	}
}

func TestLoadDatabaseList(t *testing.T) {
	tmp := writeTempYAML(t, `
databases:
  - name: "nt"
    type: "nucleotide"
    path: "/path/to/nt"
    description: "Test db"
    last_updated: "2026-01-01"
`)
	defer os.Remove(tmp)

	list, err := loadDatabaseList(tmp)
	if err != nil {
		t.Fatalf("loadDatabaseList() error = %v", err)
	}

	if len(list.Databases) != 1 {
		t.Fatalf("expected 1 database, got %d", len(list.Databases))
	}
	if list.Databases[0].Name != "nt" {
		t.Errorf("expected name 'nt', got %s", list.Databases[0].Name)
	}
}

func TestValidateDatabasePaths(t *testing.T) {
	tests := []struct {
		name    string
		list    *DatabaseList
		wantErr bool
	}{
		{"valid", &DatabaseList{[]DatabaseEntry{{Name: "nt", Type: "nucleotide", Path: "/path"}}}, false},
		{"no name", &DatabaseList{[]DatabaseEntry{{Type: "nucleotide", Path: "/path"}}}, true},
		{"invalid type", &DatabaseList{[]DatabaseEntry{{Name: "nt", Type: "invalid", Path: "/path"}}}, true},
		{"no path", &DatabaseList{[]DatabaseEntry{{Name: "nt", Type: "nucleotide"}}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDatabasePaths(tt.list)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDatabasePaths() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "helixblast-test-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}
