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
	ExitCodes  []string                `json:"exit_codes,omitempty"`
	Flags      []toolspec.ManifestFlag `json:"flags,omitempty"`
}

// exitCodeEntry is one row of the document's exit-code table: the
// symbol a leaf's exit_codes entry names, its numeric value, and what
// it means.
type exitCodeEntry struct {
	Code       int    `json:"code"`
	Symbol     string `json:"symbol"`
	Meaning    string `json:"meaning"`
	Transience string `json:"transience,omitempty"`
}

// capabilitiesDoc is the machine-readable capability surface the
// binary serves: agents parse leaves[].path to discover what they may
// invoke without scraping help text.
type capabilitiesDoc struct {
	Tool          string           `json:"tool"`
	Version       string           `json:"version"`
	SchemaVersion string           `json:"schema_version"`
	ExitCodes     []exitCodeEntry  `json:"exit_codes"`
	Leaves        []capabilityLeaf `json:"leaves"`
}

// wsmExitCodes is the whole exit-code contract, published because the
// 12fcc spec requires any code above 6 to appear in the capability
// surface — and wsm has three (8, 9, 70).
//
// The numbers are carried explicitly rather than left for the reader
// to resolve from the symbol. kit's own symbol table
// (conformance/harness) stops at PROVENANCE_MISSING and maps anything
// it does not know to 1, so a consumer resolving "PREREQUISITE"
// through kit would get the wrong number. Publishing the pair removes
// the lookup.
//
// Transience is the default for the class, not a promise for a given
// invocation: every error envelope carries its own transience field
// under --format json, and that value is the authoritative one.
var wsmExitCodes = []exitCodeEntry{
	{Code: 0, Symbol: "OK", Meaning: "success"},
	{Code: 1, Symbol: "GENERIC", Meaning: "unclassified error", Transience: "unknown"},
	{Code: 2, Symbol: "USAGE", Meaning: "bad flags or arguments", Transience: "permanent"},
	{Code: 3, Symbol: "NOT_FOUND", Meaning: "workspace, space or snapshot does not exist", Transience: "permanent"},
	{Code: 4, Symbol: "CONFLICT", Meaning: "already exists, in use, or denied by policy", Transience: "permanent"},
	{Code: ExitForbidden, Symbol: "UNAUTHORIZED", Meaning: "capability gate denied the operation, or a destructive command was refused by --confirm", Transience: "permanent"},
	{Code: ExitBackend, Symbol: "BACKEND", Meaning: "persistence failure: the store could not be opened, read or written", Transience: "unknown"},
	{Code: ExitNotConfigured, Symbol: "NOT_CONFIGURED", Meaning: "wsm has not been told something yet; the message names the command to run or the key to set", Transience: "permanent"},
	{Code: output.ExitPrerequisite, Symbol: "PREREQUISITE", Meaning: "the environment is not ready: missing binary, busy port, unreachable server", Transience: "unknown"},
	{Code: 130, Symbol: "INTERRUPTED", Meaning: "cancelled by signal", Transience: "unknown"},
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
				ExitCodes:     wsmExitCodes,
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
					ExitCodes:  mc.ExitCodes,
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
