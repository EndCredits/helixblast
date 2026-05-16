package blast

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ExecConfig struct {
	BlastPath      string
	Program        string
	Database       string
	Query          string
	NumThreads     int
	MaxTargetSeqs  int
	EValue         float64
	OutFmt         string
	AdvancedParams map[string]string
	Timeout        time.Duration
	WorkDir        string
}

func ResolveBlastPath(configPath string) (string, error) {
	if configPath != "" {
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return "", fmt.Errorf("resolve blast path: %w", err)
		}
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return "", fmt.Errorf("BLAST path does not exist: %s", absPath)
		}
		return absPath, nil
	}

	for _, dir := range strings.Split(os.Getenv("PATH"), ":") {
		candidate := filepath.Join(dir, "blastn")
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
	}

	return "", fmt.Errorf("BLAST+ not found in PATH or config; please install NCBI BLAST+ and set blast.path in config.yaml")
}

func (e *ExecConfig) BuildCommand() ([]string, error) {
	exe := e.Program
	if e.BlastPath != "" {
		exe = filepath.Join(e.BlastPath, e.Program)
	}

	args := []string{
		"-query", e.Query,
		"-db", e.Database,
		"-outfmt", e.OutFmt,
		"-num_threads", fmt.Sprintf("%d", e.NumThreads),
		"-max_target_seqs", fmt.Sprintf("%d", e.MaxTargetSeqs),
		"-evalue", fmt.Sprintf("%g", e.EValue),
	}

	for k, v := range e.AdvancedParams {
		if err := SanitizeAdvancedParam(k, v); err != nil {
			return nil, fmt.Errorf("invalid param %s: %w", k, err)
		}
		args = append(args, "-"+k, v)
	}

	return append([]string{exe}, args...), nil
}

func Execute(ctx context.Context, cfg ExecConfig) ([]Hit, error) {
	args, err := cfg.BuildCommand()
	if err != nil {
		return nil, fmt.Errorf("build command: %w", err)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = cfg.WorkDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("blast timed out after %v", duration)
		}
		return nil, fmt.Errorf("blast execution failed: %w\nstderr: %s", err, stderr.String())
	}

	hits, err := ParseOutfmt6(&stdout)
	if err != nil {
		return nil, fmt.Errorf("parse output: %w", err)
	}

	return hits, nil
}
