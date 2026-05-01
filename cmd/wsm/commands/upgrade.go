package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"hop.top/kit/go/core/upgrade"
	"hop.top/kit/go/core/upgrade/skill"
)

const wsmGitHubRepo = "hop-top/wsm"

func newChecker() *upgrade.Checker {
	return upgrade.New(
		upgrade.WithBinary("wsm", Version),
		upgrade.WithGitHub(wsmGitHubRepo),
	)
}

func newUpgradeCmd() *cobra.Command {
	var auto, quiet bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for and install updates",
		Long:  `Check for a newer version of wsm and optionally install it.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return upgrade.RunCLI(cmd.Context(), newChecker(), upgrade.CLIOptions{
				AutoUpgrade: auto,
				Quiet:       quiet,
			})
		},
	}

	cmd.Flags().BoolVar(&auto, "auto", false, "Install without prompting")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress output when already up to date")
	cmd.AddCommand(newUpgradePreambleCmd())
	return cmd
}

func newUpgradePreambleCmd() *cobra.Command {
	var auto, never, install bool

	cmd := &cobra.Command{
		Use:   "preamble",
		Short: "Print the upgrade preamble fragment for skill files",
		Long: `Print a markdown preamble fragment for embedding in WSM skill files.
Agents read this to know how to self-upgrade wsm before executing tasks.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			level := skill.SnoozeOnce
			if auto {
				level = skill.SnoozeNever
			} else if never {
				level = skill.SnoozeAlways
			}

			preamble := skill.Generate(skill.PreambleOptions{
				BinaryName: "wsm",
				Snooze:     level,
			})

			if install {
				return installPreamble(preamble)
			}

			fmt.Print(preamble)
			return nil
		},
	}

	cmd.Flags().BoolVar(&auto, "auto", false, "Emit auto-upgrade (SnoozeNever) variant")
	cmd.Flags().BoolVar(&never, "never", false, "Emit check-only (SnoozeAlways) variant")
	cmd.Flags().BoolVar(&install, "install", false, "Write preamble to ~/.config/wsm/skills/")
	return cmd
}

func installPreamble(preamble string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("upgrade preamble: %w", err)
	}

	dir := configDir + "/wsm/skills"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("upgrade preamble: mkdir: %w", err)
	}

	path := dir + "/upgrade-preamble.md"
	if err := os.WriteFile(path, []byte(preamble), 0o600); err != nil {
		return fmt.Errorf("upgrade preamble: write: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Installed upgrade preamble → %s\n", path)
	return nil
}
