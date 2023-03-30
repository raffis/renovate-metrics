package main

import (
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:           "renovate-metrics",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return nil
		},
	}
	fileArg = "-"
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&fileArg, "file", "f", fileArg, "Path to log file (Defaults to stdin)")
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		panic(err)
	}
}
