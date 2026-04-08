package guesscmd

import (
	"fmt"
	"os"

	"guessapi/internal/config"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "guessapi",
	Short: "Guess the Prompt Backend API",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(config.LoadConfig)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(scaffoldCmd)
	rootCmd.AddCommand(scaffoldImagesCmd)
	rootCmd.AddCommand(cleanupDuplicatesCmd)
	rootCmd.AddCommand(cleanupOrphanImagesCmd)
}
