package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"hop.top/kit/go/ai/toolspec"
	speccli "hop.top/kit/go/ai/toolspec/cli"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// capabilitiesSchemaVersion is the CLI-surface schema version the
// capabilities document claims. Distinct from the binary semver: it
// evolves when the introspection shape changes.
const capabilitiesSchemaVersion = "1.0"

// capabilityLeaf is one invocable leaf in the capabilities document:
// full command path, leaf name, contract annotations, and the leaf's
// local flag surface.
type capabilityLeaf struct {
	Path       string                  `json:"path"`
	Name       string                  `json:"name"`
	Short      string                  `json:"short,omitempty"`
	SideEffect string                  `json:"side_effect,omitempty"`
	Idempotent string                  `json:"idempotent,omitempty"`
	DryRun     bool                    `json:"dry_run_supported"`
	Flags      []toolspec.ManifestFlag `json:"flags,omitempty"`
}

// capabilitiesDoc is the machine-readable capability surface the
// binary serves: agents parse leaves[].path to discover what they may
// invoke without scraping help text.
type capabilitiesDoc struct {
	Tool          string           `json:"tool"`
	Version       string           `json:"version"`
	SchemaVersion string           `json:"schema_version"`
	Leaves        []capabilityLeaf `json:"leaves"`
}

// capabilityRow is the flat projection rendered by the table-family
// formats.
type capabilityRow struct {
	Path        string `table:"PATH"        json:"path"`
	SideEffect  string `table:"SIDE-EFFECT" json:"side_effect"`
	Idempotent  string `table:"IDEMPOTENT"  json:"idempotent"`
	Description string `table:"DESCRIPTION" json:"description"`
}

// newCapabilitiesCmd mounts the capability introspection leaf. The
// projection reuses kit's toolspec walker so annotations, deprecation
// filtering, and flag discovery stay kit-canonical; only the emitted
// shape differs — a flat leaves list keyed by string path. The
// document mirrors the live command tree, so a leaf listed here is
// invocable as printed.
func newCapabilitiesCmd(root *cli.Root) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "Emit the machine-readable command surface",
		Long: `Describe every invocable leaf of the wsm CLI as structured data:
full command path, leaf name, side-effect and idempotency contract,
dry-run support, and per-leaf flags. Agents consume this instead of
scraping help text; humans get an aligned table.

Read-only. --format json emits the full document; table renders the
leaf listing. The document mirrors the live command tree, so a leaf
listed here is invocable as printed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := speccli.EmitManifest(root, capabilitiesSchemaVersion)
			doc := capabilitiesDoc{
				Tool:          m.Tool,
				Version:       m.Version,
				SchemaVersion: m.SchemaVersion,
				Leaves:        make([]capabilityLeaf, 0, len(m.Commands)),
			}
			for _, mc := range m.Commands {
				if len(mc.Path) == 0 {
					continue
				}
				doc.Leaves = append(doc.Leaves, capabilityLeaf{
					Path:       strings.Join(mc.Path, " "),
					Name:       mc.Path[len(mc.Path)-1],
					Short:      mc.Short,
					SideEffect: mc.SideEffect,
					Idempotent: mc.Idempotent,
					DryRun:     mc.DryRunSupported,
					Flags:      mc.Flags,
				})
			}

			format := output.Table
			if f := cmd.Flag("format"); f != nil && f.Value.String() != "" {
				format = f.Value.String()
			}
			switch format {
			case output.JSON, output.YAML:
				// The nested document is the machine contract.
				return output.Render(cmd.OutOrStdout(), format, doc)
			default:
				rows := make([]capabilityRow, 0, len(doc.Leaves))
				for _, l := range doc.Leaves {
					rows = append(rows, capabilityRow{
						Path:        l.Path,
						SideEffect:  l.SideEffect,
						Idempotent:  l.Idempotent,
						Description: l.Short,
					})
				}
				return output.Dispatch(cmd, nil, rows)
			}
		},
	}

	// Serializes the live command tree. No config read for the
	// projection itself, no filesystem write, no network.
	cli.SetSideEffect(cmd, cli.SideEffectRead)
	cli.SetIdempotency(cmd, cli.IdempotencyYes)
	cli.SetTopLevelVerb(cmd)

	return cmd
}
