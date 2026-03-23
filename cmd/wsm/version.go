package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	var jsonOut, short bool

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

			if jsonOut {
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

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output version info as JSON")
	cmd.Flags().BoolVar(&short, "short", false, "Output only the version number")
	return cmd
}
