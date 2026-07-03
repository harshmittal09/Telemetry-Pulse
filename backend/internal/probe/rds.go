// Package probe — rds.go
//
// RDSProber queries an AWS RDS cluster for:
//   - Instance status and availability
//   - Automated (continuous) backup status and the most recent restore window
//   - Snapshot inventory — count, latest completion time, encrypted flag
//   - Estimated backup lag (time since last automated snapshot completed)
//
// Authentication: uses the default AWS credential chain
// (env vars → ~/.aws/credentials → EC2/ECS IAM role → etc.).
// Configure via environment variables:
//
//	AWS_REGION            (e.g. "us-east-1")
//	RDS_CLUSTER_ID        (DB cluster or instance identifier)
//
// The probe publishes results on the same Redis pipeline as the network
// and system probes, using EndpointID "rds-<cluster-id>".
package probe

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/telemetrypulse/backend/pkg/models"
)

// RDSProber wraps the AWS RDS client and queries a specific cluster.
type RDSProber struct {
	client    *rds.Client
	clusterID string
}

// NewRDSProber loads the AWS default config and creates an RDS client.
// Returns an error if the AWS SDK cannot be initialised (e.g., no credentials).
func NewRDSProber(ctx context.Context) (*RDSProber, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1" // safe default; overridden by env
	}
	clusterID := os.Getenv("RDS_CLUSTER_ID")
	if clusterID == "" {
		return nil, fmt.Errorf("rds probe: RDS_CLUSTER_ID environment variable not set")
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("rds probe: load AWS config: %w", err)
	}

	return &RDSProber{
		client:    rds.NewFromConfig(cfg),
		clusterID: clusterID,
	}, nil
}

// ProbeRDS queries the RDS cluster and returns an RDSMetrics snapshot.
// It makes two API calls:
//  1. DescribeDBClusters — cluster status and continuous backup window.
//  2. DescribeDBClusterSnapshots — automated snapshot inventory.
func (p *RDSProber) ProbeRDS(ctx context.Context) (models.RDSMetrics, error) {
	m := models.RDSMetrics{
		ClusterID: p.clusterID,
		Timestamp: time.Now().UTC(),
	}

	// ── 1. Cluster status & continuous backup details ──────────────────────
	clusterOut, err := p.client.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(p.clusterID),
	})
	if err != nil {
		return m, fmt.Errorf("rds probe: DescribeDBClusters: %w", err)
	}
	if len(clusterOut.DBClusters) == 0 {
		return m, fmt.Errorf("rds probe: cluster %q not found", p.clusterID)
	}

	cluster := clusterOut.DBClusters[0]
	m.Status = aws.ToString(cluster.Status)
	m.Engine = aws.ToString(cluster.Engine)
	m.EngineVersion = aws.ToString(cluster.EngineVersion)
	m.MultiAZ = aws.ToBool(cluster.MultiAZ)
	m.BackupRetentionDays = int(aws.ToInt32(cluster.BackupRetentionPeriod))
	m.PreferredBackupWindow = aws.ToString(cluster.PreferredBackupWindow)

	// EarliestRestorableTime reflects the start of the continuous backup log.
	if cluster.EarliestRestorableTime != nil {
		m.EarliestRestorableTime = *cluster.EarliestRestorableTime
	}
	// LatestRestorableTime reflects the most recent point available for PITR.
	if cluster.LatestRestorableTime != nil {
		m.LatestRestorableTime = *cluster.LatestRestorableTime
		// Backup lag = now − latest restorable time. A healthy cluster's lag
		// should be small (typically < 5 minutes for Aurora).
		m.BackupLagSeconds = time.Since(*cluster.LatestRestorableTime).Seconds()
	}

	// ── 2. Automated snapshot inventory ───────────────────────────────────
	snapOut, err := p.client.DescribeDBClusterSnapshots(ctx, &rds.DescribeDBClusterSnapshotsInput{
		DBClusterIdentifier: aws.String(p.clusterID),
		SnapshotType:        aws.String("automated"),
	})
	if err != nil {
		// Non-fatal: log and continue with what we have.
		m.SnapshotQueryError = err.Error()
	} else {
		m.AutomatedSnapshotCount = len(snapOut.DBClusterSnapshots)
		for _, snap := range snapOut.DBClusterSnapshots {
			if snap.SnapshotCreateTime != nil {
				if m.LatestSnapshotTime.IsZero() || snap.SnapshotCreateTime.After(m.LatestSnapshotTime) {
					m.LatestSnapshotTime = *snap.SnapshotCreateTime
					m.LatestSnapshotStatus = aws.ToString(snap.Status)
					m.LatestSnapshotEncrypted = aws.ToBool(snap.StorageEncrypted)
					m.LatestSnapshotSizeGB = aws.ToInt32(snap.AllocatedStorage)
				}
			}
			// Count snapshots that completed successfully.
			if aws.ToString(snap.Status) == "available" {
				m.CompletedSnapshotCount++
			}
		}
	}

	return m, nil
}

// RDSDispatcher is a dedicated probe loop for RDS metrics.
type RDSDispatcher struct {
	prober   *RDSProber
	callback RDSResultCallback
	interval time.Duration
}

// RDSResultCallback receives each RDSMetrics snapshot.
type RDSResultCallback func(m models.RDSMetrics)

// NewRDSDispatcher creates an RDSDispatcher.
func NewRDSDispatcher(prober *RDSProber, interval time.Duration, cb RDSResultCallback) *RDSDispatcher {
	return &RDSDispatcher{
		prober:   prober,
		callback: cb,
		interval: interval,
	}
}

// Start launches the RDS probe loop in a background goroutine.
// Runs until ctx is cancelled.
func (d *RDSDispatcher) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m, err := d.prober.ProbeRDS(ctx)
				if err != nil {
					// Log error as a degraded metric rather than crashing the loop.
					m.ProbeError = err.Error()
				}
				d.callback(m)
			}
		}
	}()
}
