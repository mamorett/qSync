package cli

import (
	"fmt"

	"github.com/yourorg/qsync/internal/config"
	"github.com/yourorg/qsync/internal/conflict"
	"github.com/yourorg/qsync/internal/planner"
	"github.com/yourorg/qsync/internal/rsyncx"
	"github.com/yourorg/qsync/internal/snapshot"
)

// buildContext holds the manifests gathered for planning.
type buildContext struct {
	local  *snapshot.Manifest
	remote *snapshot.Manifest
	synced *snapshot.Manifest
	plan   *planner.Plan
}

// gatherAndPlan performs: local scan → fetch remote manifest → load synced →
// conflict detection → build plan → persist local/remote manifests. It returns
// scan errors separately so callers can decide severity.
func gatherAndPlan(cfg *config.Config, direction planner.Direction) (*buildContext, []snapshot.ScanError, error) {
	res, err := snapshot.Scan(cfg.Target.Path, cfg.Ignore)
	if err != nil {
		return nil, nil, err
	}
	local := res.Manifest

	remote, err := fetchRemoteManifest(cfg, false)
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

	conflicts := conflict.Detect(synced, local, remote)
	plan := planner.Build(direction, synced, local, remote, conflicts)
	src, dst := rsyncx.Describe(direction, cfg)
	plan.Source, plan.Dest = src, dst

	return &buildContext{local: local, remote: remote, synced: synced, plan: plan}, res.Errors, nil
}
