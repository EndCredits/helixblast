package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type DatabaseEntry struct {
	Name        string          `yaml:"name"`
	Type        string          `yaml:"type"`
	Path        string          `yaml:"path"`
	Description string          `yaml:"description"`
	LastUpdated string          `yaml:"last_updated"`
	Transcript  TranscriptConfig `yaml:"transcript"`
}

type TranscriptConfig struct {
	IndexPath string `yaml:"index_path"`
	FastaDir  string `yaml:"fasta_dir"`
	FastaFile string `yaml:"fasta_file"`
}

type DatabaseList struct {
	Databases []DatabaseEntry `yaml:"databases"`
}

type DatabaseManager struct {
	current  atomic.Value
	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
}

func NewDatabaseManager(configPath string) (*DatabaseManager, error) {
	list, err := loadDatabaseList(configPath)
	if err != nil {
		return nil, fmt.Errorf("load database config: %w", err)
	}

	dm := &DatabaseManager{
		stopCh: make(chan struct{}),
	}
	dm.current.Store(list)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	dm.watcher = watcher

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		absPath = configPath
	}

	dir := filepath.Dir(absPath)
	if err := watcher.Add(dir); err != nil {
		return nil, fmt.Errorf("watch directory %s: %w", dir, err)
	}

	go dm.watchLoop(absPath)

	return dm, nil
}

func (dm *DatabaseManager) List() []DatabaseEntry {
	v := dm.current.Load()
	if v == nil {
		return nil
	}
	list := v.(*DatabaseList)
	out := make([]DatabaseEntry, len(list.Databases))
	copy(out, list.Databases)
	return out
}

func (dm *DatabaseManager) Lookup(name string) (*DatabaseEntry, error) {
	for _, db := range dm.List() {
		if db.Name == name {
			return &db, nil
		}
	}
	return nil, fmt.Errorf("database not found: %s", name)
}

func (dm *DatabaseManager) Stop() {
	close(dm.stopCh)
	if dm.watcher != nil {
		dm.watcher.Close()
	}
}

func (dm *DatabaseManager) watchLoop(targetPath string) {
	for {
		select {
		case <-dm.stopCh:
			return
		case event, ok := <-dm.watcher.Events:
			if !ok {
				return
			}
			absEvent, err := filepath.Abs(event.Name)
			if err != nil {
				continue
			}
			absTarget, err := filepath.Abs(targetPath)
			if err != nil {
				continue
			}
			if absEvent != absTarget {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				list, err := loadDatabaseList(targetPath)
				if err != nil {
					log.Printf("[helixblast] Failed to reload databases.yaml: %v", err)
					continue
				}
				if err := validateDatabasePaths(list); err != nil {
					log.Printf("[helixblast] Database validation failed, keeping old config: %v", err)
					continue
				}
				dm.current.Store(list)
				log.Printf("[helixblast] Databases reloaded: %d database(s)", len(list.Databases))
			}
		case err, ok := <-dm.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[helixblast] fsnotify error: %v", err)
		}
	}
}

func loadDatabaseList(path string) (*DatabaseList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var list DatabaseList
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	if err := validateDatabasePaths(&list); err != nil {
		return nil, err
	}

	return &list, nil
}

func validateDatabasePaths(list *DatabaseList) error {
	for i, db := range list.Databases {
		if db.Name == "" {
			return fmt.Errorf("database[%d]: name is required", i)
		}
		if db.Type != "nucleotide" && db.Type != "protein" {
			return fmt.Errorf("database[%d]: type must be 'nucleotide' or 'protein'", i)
		}
		if db.Path == "" {
			return fmt.Errorf("database[%d]: path is required", i)
		}
	}
	return nil
}
