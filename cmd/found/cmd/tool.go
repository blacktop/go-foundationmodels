package cmd

import (
	"github.com/spf13/cobra"
)

// toolCmd represents the tool command
var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Interact with Foundation Models using tools",
}

func init() {
	rootCmd.AddCommand(toolCmd)
}
