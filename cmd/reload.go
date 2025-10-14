package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// 定义接口，方便 mock
type ReloadClient interface {
	Reload(ctx context.Context) error
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReload(cmd.Context(), cli, cmd.OutOrStdout())
	},
}

// runReload 提取的业务逻辑，方便测试
func runReload(ctx context.Context, client ClientInterface, out io.Writer) error {
	if err := client.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload: %w", err)
	}
	fmt.Fprintln(out, "✓ Configuration reloaded successfully")
	return nil
}

func init() {
	rootCmd.AddCommand(reloadCmd)
}
