package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "DEV"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the current version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("bobr %s\n", Version)
	},
}
