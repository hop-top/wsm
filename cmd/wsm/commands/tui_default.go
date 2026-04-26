//go:build !charmui

package commands

import (
	"context"

	"hop.top/wsm/pkg/workspace"
)

func runTUI(_ context.Context, _ *appContext, _ globalOptions) error {
	return workspace.NewErrorUsage("tui is unavailable in this build; rebuild with -tags charmui -modfile=go.charm.mod")
}
