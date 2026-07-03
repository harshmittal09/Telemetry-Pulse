// cmd/archiver/main.go — TelemetryPulse S3 Log Archiver CLI
//
// Usage:
//
//	archiver [flags]
//
// Flags:
//
//	-source   string   Label for the data source (default "manual")
//	-file     string   Path to the log file to archive. If "-", reads stdin.
//
// Required environment variables:
//
//	AWS_REGION     AWS region where the S3 bucket lives (default: us-east-1)
//	S3_BUCKET      Target S3 bucket name
//
// Optional environment variables:
//
//	S3_KEY_PREFIX  Prefix for S3 object keys (default: telemetrypulse/archive)
//	AWS_PROFILE    Named AWS credentials profile
//
// Examples:
//
//	# Archive a specific log file
//	S3_BUCKET=my-bucket archiver -source network-telemetry -file /var/log/telemetry.log
//
//	# Pipe stdin directly (e.g. from a cron job)
//	journalctl -u telemetrypulse | S3_BUCKET=my-bucket archiver -source service-logs -file -
//
//	# Archive all .log files in a directory
//	for f in /var/log/telemetrypulse/*.log; do
//	    S3_BUCKET=my-bucket archiver -source "$(basename $f .log)" -file "$f"
//	done
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/telemetrypulse/backend/internal/archive"
)

func main() {
	// ── Flags ──────────────────────────────────────────────────────────────
	source := flag.String("source", "manual", "Label for the archived data source")
	filePath := flag.String("file", "-", "Path to the file to archive. Use \"-\" for stdin")
	flag.Parse()

	// ── Structured logger ──────────────────────────────────────────────────
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Context with graceful interrupt ────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── Open input ─────────────────────────────────────────────────────────
	var src io.Reader
	if *filePath == "-" {
		slog.Info("Reading from stdin")
		src = os.Stdin
	} else {
		f, err := os.Open(*filePath)
		if err != nil {
			fatal("Failed to open input file", err)
		}
		defer f.Close()
		info, _ := f.Stat()
		slog.Info("Reading input file", "path", *filePath, "size_bytes", info.Size())
		src = f
	}

	// ── Create archiver ────────────────────────────────────────────────────
	slog.Info("Initialising S3 archiver",
		"bucket", os.Getenv("S3_BUCKET"),
		"region", getEnvOrDefault("AWS_REGION", "us-east-1"),
		"prefix", getEnvOrDefault("S3_KEY_PREFIX", "telemetrypulse/archive"),
	)

	archiver, err := archive.NewArchiver(ctx)
	if err != nil {
		fatal("Failed to create S3 archiver", err)
	}

	// ── Upload ─────────────────────────────────────────────────────────────
	start := time.Now()
	uri, err := archiver.ArchiveStream(ctx, *source, src)
	if err != nil {
		fatal("Archive upload failed", err)
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	slog.Info("Archive complete", "uri", uri, "elapsed", elapsed)
	fmt.Printf("\n✓ Archived successfully in %s\n  URI: %s\n", elapsed, uri)
}

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
