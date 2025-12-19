package cli

import (
	"fmt"

	"github.com/azuradara/bobr/internal/cache"
	"github.com/azuradara/bobr/internal/config"
	"github.com/spf13/cobra"
)

var purgeCmd = &cobra.Command{
	Use:   "purge [object-name-or-prefix]",
	Short: "Purge objects from cache",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPurge(args[0])
	},
}

func runPurge(target string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	c, err := cache.New(cfg.Cache)
	if err != nil {
		return fmt.Errorf("failed to open cache: %w", err)
	}
	defer func() { _ = c.Close() }()

	fmt.Printf("[*] purging objects matching '%s'...\n", target)

	count, err := c.Purge(target)
	if err != nil {
		fmt.Println("[fail]")

		return fmt.Errorf("failed to purge cache: %w", err)
	}

	if count == 0 {
		fmt.Printf("[*] no objects found matching '%s'.\n", target)
	} else {
		fmt.Printf("[*] successfully purged %d object(s).\n", count)
	}

	return nil
}
