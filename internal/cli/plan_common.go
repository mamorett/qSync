package cli

import (
	"context"
	"fmt"

	"github.com/mamorett/qsync/internal/config"
	"github.com/mamorett/qsync/internal/conflict"
	"github.com/mamorett/qsync/internal/planner"
	"github.com/mamorett/qsync/internal/rsyncx"
	"github.com/mamorett/qsync/internal/snapshot"
)

// buildContext holds the manifests gathered for planning.
type buildContext struct {
	local  *snapshot.Manifest
	remote *snapshot.Manifest
	synced *snapshot.Manifest
	plan   *planner.Plan
}

// gatherAndPlan performs: local scan → fetch remote manifest → load synced →
// conflict detection → build plan → persist local/remote manifests.
func gatherAndPlan(cfg *config.Config, direction planner.Direction, vfat bool) (*buildContext, []snapshot.ScanError, error) {
	return gatherAndPlanContext(context.Background(), cfg, direction, vfat)
}

func gatherAndPlanContext(ctx context.Context, cfg *config.Config, direction planner.Direction, vfat bool) (*buildContext, []snapshot.ScanError, error) {
	res, err := snapshot.Scan(cfg.Target.Path, cfg.Ignore)
	if err != nil {
		return nil, nil, err
	}
	local := res.Manifest

	remote, err := fetchRemoteManifestContext(ctx, cfg, false)
	if err != nil {
		return nil, res.Errors, err
	}

	sp := stateFor(cfg.Target.Path)
	synced, _, err := loadSynced(sp)
	if err != nil {
		return nil, res.Errors, err
	}

	// Persist refreshed state manifests (allowed: state refresh, not mutation).
	if err := local.Save(sp.local); err != nil {
		return nil, res.Errors, fmt.Errorf("save local manifest: %w", err)
	}
	if err := remote.Save(sp.remote); err != nil {
		return nil, res.Errors, fmt.Errorf("save remote manifest: %w", err)
	}

	conflicts := conflict.DetectVFAT(synced, local, remote, vfat)
	plan := planner.BuildVFAT(direction, synced, local, remote, conflicts, vfat)
	src, dst := rsyncx.Describe(direction, cfg)
	plan.Source, plan.Dest = src, dst

	return &buildContext{local: local, remote: remote, synced: synced, plan: plan}, res.Errors, nil
}
