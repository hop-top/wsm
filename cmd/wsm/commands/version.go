package commands

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"hop.top/kit/go/console/cli"
)

func newVersionCmd() *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  `Print detailed version information including build metadata.`,
		Run: func(cmd *cobra.Command, _ []string) {
			if short {
				fmt.Println(Version)
				return
			}

			commit := CommitSHA
			if len(commit) > 7 {
				commit = commit[:7]
			}

			// Read the persistent --format flag (registered by kit/cli).
			format, _ := cmd.Flags().GetString("format")
			if format == "json" || format == "yaml" {
				info := map[string]string{
					"version":   Version,
					"commit":    CommitSHA,
					"goVersion": runtime.Version(),
					"os":        runtime.GOOS,
					"arch":      runtime.GOARCH,
				}
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return
			}

			if commit != "" {
				fmt.Printf("wsm v%s (%s)\n", Version, commit)
			} else {
				fmt.Printf("wsm v%s\n", Version)
			}
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "Output only the version number")
	cli.SetTopLevelVerb(cmd)
	return cmd
}
