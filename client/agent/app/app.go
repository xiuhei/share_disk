// Package app exposes the cross-platform Agent lifecycle to desktop launchers.
// Product-specific launchers own paths, native integration and packaging while
// the storage and transfer implementation remains shared with the Ubuntu Agent.
package app

import (
	"context"

	"github.com/share-disk/share-disk/client/ubuntu/internal/agent"
	"github.com/share-disk/share-disk/internal/config"
)

// Run creates and serves an Agent until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config, migrationsDir string) error {
	a, err := agent.New(cfg, migrationsDir)
	if err != nil {
		return err
	}
	defer a.Close()
	return a.Run(ctx)
}
