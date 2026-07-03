// Package archive implements the S3 log archiving pipeline for TelemetryPulse.
//
// Responsibilities:
//  1. Accept an io.Reader of raw log/metrics data (JSON lines, plain text, etc.)
//  2. Compress it on-the-fly using gzip (no intermediate tmp file required)
//  3. Upload the compressed stream to an S3 bucket via the AWS SDK v2
//     multi-part upload manager.
//
// Object key format:
//
//	telemetrypulse/archive/<YYYY>/<MM>/<DD>/<source>-<RFC3339Nano>.log.gz
//
// Configuration is driven entirely by environment variables:
//
//	AWS_REGION           (e.g. "us-east-1")
//	S3_BUCKET            (e.g. "telemetrypulse-archives")
//	S3_KEY_PREFIX        (optional, default "telemetrypulse/archive")
//
// Authentication uses the default AWS credential chain
// (env → shared credentials file → EC2/ECS IAM role).
package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/telemetrypulse/backend/pkg/models"
)

// ---------------------------------------------------------------------------
// Archiver
// ---------------------------------------------------------------------------

// Archiver holds a configured S3 upload manager and target bucket details.
type Archiver struct {
	uploader  *manager.Uploader
	bucket    string
	keyPrefix string
}

// NewArchiver loads the AWS SDK configuration and creates an Archiver.
// Returns an error if the SDK cannot be initialised or S3_BUCKET is not set.
func NewArchiver(ctx context.Context) (*Archiver, error) {
	region := getEnv("AWS_REGION", "us-east-1")
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("archive: S3_BUCKET environment variable not set")
	}
	keyPrefix := getEnv("S3_KEY_PREFIX", "telemetrypulse/archive")

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("archive: load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		// 10 MiB parts — safe for gzip streams of any length.
		u.PartSize = 10 * 1024 * 1024
		// 3 concurrent upload goroutines.
		u.Concurrency = 3
	})

	return &Archiver{
		uploader:  uploader,
		bucket:    bucket,
		keyPrefix: strings.TrimRight(keyPrefix, "/"),
	}, nil
}

// ---------------------------------------------------------------------------
// Core upload method
// ---------------------------------------------------------------------------

// ArchiveStream compresses data from src on-the-fly and uploads the resulting
// gzip stream to S3. The S3 object key encodes the current date and the
// supplied source label (e.g. "network", "system", "rds-backup").
//
//	key = <keyPrefix>/<YYYY>/<MM>/<DD>/<source>-<timestamp>.log.gz
//
// Returns the final S3 URI (s3://<bucket>/<key>) on success.
func (a *Archiver) ArchiveStream(ctx context.Context, source string, src io.Reader) (string, error) {
	now := time.Now().UTC()
	key := fmt.Sprintf("%s/%s/%s/%s/%s-%s.log.gz",
		a.keyPrefix,
		now.Format("2006"),
		now.Format("01"),
		now.Format("02"),
		sanitiseSource(source),
		now.Format(time.RFC3339Nano),
	)

	// Compress src → gzip pipe without buffering the entire payload in RAM.
	pr, pw := io.Pipe()
	gzErrs := make(chan error, 1)

	go func() {
		gz := gzip.NewWriter(pw)
		gz.Header = gzip.Header{
			Name:    source + ".log",
			ModTime: now,
			Comment: "TelemetryPulse archive",
		}
		_, copyErr := io.Copy(gz, src)
		closeErr := gz.Close()
		pw.CloseWithError(coalesceErr(copyErr, closeErr))
		gzErrs <- coalesceErr(copyErr, closeErr)
	}()

	result, err := a.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:          aws.String(a.bucket),
		Key:             aws.String(key),
		Body:            pr,
		ContentType:     aws.String("application/gzip"),
		ContentEncoding: aws.String("gzip"),
		// Server-side encryption using the bucket's default KMS key.
		ServerSideEncryption: "aws:kms",
		// Tagging for lifecycle policies.
		Tagging: aws.String("project=telemetrypulse&source=" + sanitiseSource(source)),
	})

	// Always drain the gzip goroutine error.
	gzErr := <-gzErrs

	if err != nil {
		return "", fmt.Errorf("archive: S3 upload failed: %w", err)
	}
	if gzErr != nil {
		return "", fmt.Errorf("archive: gzip compression failed: %w", gzErr)
	}

	uri := fmt.Sprintf("s3://%s/%s", a.bucket, key)
	slog.Info("Archive uploaded",
		"uri", uri,
		"etag", aws.ToString(result.ETag),
		"upload_id", result.UploadID,
	)
	return uri, nil
}

// ---------------------------------------------------------------------------
// Convenience helpers — archive structured metric slices
// ---------------------------------------------------------------------------

// ArchiveTelemetryPayloads serialises a batch of TelemetryPayload records as
// JSON Lines and ships the gzip-compressed stream to S3.
func (a *Archiver) ArchiveTelemetryPayloads(ctx context.Context, payloads []models.TelemetryPayload) (string, error) {
	r, err := jsonLinesReader(payloads)
	if err != nil {
		return "", fmt.Errorf("archive: serialise telemetry payloads: %w", err)
	}
	return a.ArchiveStream(ctx, "network-telemetry", r)
}

// ArchiveSystemMetrics serialises a batch of SystemMetrics records as JSON
// Lines and ships the gzip-compressed stream to S3.
func (a *Archiver) ArchiveSystemMetrics(ctx context.Context, metrics []models.SystemMetrics) (string, error) {
	r, err := jsonLinesReader(metrics)
	if err != nil {
		return "", fmt.Errorf("archive: serialise system metrics: %w", err)
	}
	return a.ArchiveStream(ctx, "system-metrics", r)
}

// ArchiveRDSMetrics serialises a batch of RDSMetrics records as JSON Lines
// and ships the gzip-compressed stream to S3.
func (a *Archiver) ArchiveRDSMetrics(ctx context.Context, metrics []models.RDSMetrics) (string, error) {
	r, err := jsonLinesReader(metrics)
	if err != nil {
		return "", fmt.Errorf("archive: serialise RDS metrics: %w", err)
	}
	return a.ArchiveStream(ctx, "rds-backup-metrics", r)
}

// ---------------------------------------------------------------------------
// JSON Lines helper
// ---------------------------------------------------------------------------

// jsonLinesReader encodes a slice of any JSON-serialisable type as JSON Lines
// (one compact JSON object per line, newline-delimited) and returns an
// io.Reader backed by an in-memory buffer. For production use at scale this
// could be replaced with a streaming encoder to avoid the full allocation.
func jsonLinesReader[T any](items []T) (io.Reader, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return nil, err
		}
	}
	return &buf, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// sanitiseSource replaces path-unsafe characters so the source label is safe
// to embed in an S3 key.
func sanitiseSource(s string) string {
	return strings.NewReplacer(" ", "-", "/", "-", ":", "-").Replace(s)
}

// coalesceErr returns the first non-nil error, or nil if all are nil.
func coalesceErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
