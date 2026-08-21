package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EndCredits/helixblast/internal/api"
	"github.com/EndCredits/helixblast/internal/blast"
	"github.com/EndCredits/helixblast/internal/config"
	"github.com/EndCredits/helixblast/internal/janitor"
	"github.com/EndCredits/helixblast/internal/storage"
	"github.com/EndCredits/helixblast/internal/worker"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	logger := log.New(os.Stdout, "[helixblast] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("HelixBLAST v0.1.0 starting")

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}

	resources := cfg.ComputeResources()
	logger.Printf("Resource: max_jobs=%d cpu_per_job=%d actual_concurrent=%d degraded=%v",
		resources.MaxJobs, resources.CPUPerJob, resources.ActualConcurrent, resources.Degraded)
	if resources.Degraded {
		logger.Printf("WARNING: System running in degraded mode: %s", resources.DegradedReason)
	}

	logger.Printf("Storage backend: %s", cfg.Storage.Type)

	var store storage.Store
	switch cfg.Storage.Type {
	case "s3":
		s3Store, err := storage.NewS3Store(cfg.S3.Endpoint, cfg.S3.Bucket, cfg.S3.AccessKey, cfg.S3.SecretKey)
		if err != nil {
			logger.Fatalf("Failed to create S3 store: %v", err)
		}
		store = s3Store
	default:
		localStore, err := storage.NewLocalStore(cfg.Storage.DataDir)
		if err != nil {
			logger.Fatalf("Failed to create local store: %v", err)
		}
		store = localStore
	}

	blastPath, err := blast.ResolveBlastPath(cfg.Blast.Path)
	if err != nil {
		logger.Fatalf("Failed to resolve BLAST+ path: %v", err)
	}
	logger.Printf("BLAST+ path: %s", blastPath)

	whitelist, err := blast.BuildWhitelist(blastPath)
	if err != nil {
		logger.Printf("WARNING: Failed to build param whitelist: %v (all params allowed)", err)
		whitelist = nil
	} else {
		logger.Printf("BLAST param whitelist: %d params", whitelist.Len())
	}

	dm, err := config.NewDatabaseManager(cfg.Database.ConfigPath)
	if err != nil {
		logger.Fatalf("Failed to load database config: %v", err)
	}
	defer dm.Stop()

	dbs := dm.List()
	logger.Printf("Loaded %d database(s)", len(dbs))
	for _, db := range dbs {
		logger.Printf("  - %s (%s): %s", db.Name, db.Type, db.Description)
	}

	execFn := func(ctx context.Context, job *worker.Job, dbName string) ([]blast.Hit, error) {
		dbEntry, err := dm.Lookup(dbName)
		if err != nil {
			return nil, fmt.Errorf("resolve database: %w", err)
		}
		tmpFile, err := os.CreateTemp("", "helixblast-query-*.fa")
		if err != nil {
			return nil, fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(job.FastA); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("write temp file: %w", err)
		}
		tmpFile.Close()

		workDir, err := os.MkdirTemp("", "helixblast-work-")
		if err != nil {
			return nil, fmt.Errorf("create work dir: %w", err)
		}
		defer os.RemoveAll(workDir)

		params := make(map[string]string)
		for key, val := range job.AdvancedParams {
			if whitelist != nil && !whitelist.IsAllowed(key) {
				return nil, fmt.Errorf("invalid blast parameter: -%s", key)
			}
			params[key] = val
		}

		return blast.Execute(ctx, blast.ExecConfig{
			BlastPath:      blastPath,
			Program:        job.Program,
			Database:       dbEntry.Path,
			Query:          tmpFile.Name(),
			NumThreads:     cfg.Blast.CPUPerJob,
			MaxTargetSeqs:  5000,
			EValue:         10,
			OutFmt:         "6 qseqid sseqid pident qcovs evalue bitscore qstart qend sstart send qseq sseq",
			AdvancedParams: params,
			Timeout:        2 * time.Hour,
			WorkDir:        workDir,
		})
	}

	resultTTL := time.Duration(cfg.Storage.ResultTTLHours) * time.Hour
	pool := worker.NewPool(resources.ActualConcurrent, cfg.Blast.MaxJobs, execFn, resultTTL)
	logger.Printf("Worker pool: %d concurrent workers, %d max queue, registry TTL %dh",
		resources.ActualConcurrent, cfg.Blast.MaxJobs, cfg.Storage.ResultTTLHours)

	jan := janitor.New(store, cfg.Storage.ResultTTLHours)
	jan.Start()
	defer jan.Stop()

	srv := api.NewServer(cfg, pool, dm, whitelist)

	httpServer := &http.Server{
		Addr:    srv.ListenAddr(),
		Handler: srv,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Println("Shutting down...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Printf("HTTP shutdown error: %v", err)
		}

		pool.Stop()
		logger.Println("Shutdown complete")
		os.Exit(0)
	}()

	logger.Printf("Listening on :%d", cfg.Server.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Server error: %v", err)
	}
}
