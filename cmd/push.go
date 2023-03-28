package main

import (
	"os"

	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/raffis/renovate-metrics/pkg/parser"
	"github.com/spf13/cobra"
)

var (
	prometheusArg = "http://localhost:9091"
)

func init() {
	pushCmd := &cobra.Command{
		Use: "push",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var file *os.File

			if fileArg == "-" {
				file = os.Stdin
			} else {
				f, err := os.Open(fileArg)
				if err != nil {
					return err
				}

				defer f.Close()
				file = f
			}

			parser := parser.NewParser(file)
			collectors, err := parser.Parse()
			if err != nil {
				return err
			}

			client := push.New(prometheusArg, "renovate")

			for repository, collector := range collectors {
				client.Grouping("repository", repository)

				if err := client.Delete(); err != nil {
					return err
				}

				if err := client.Collector(collector).Push(); err != nil {
					return err
				}
			}

			return err
		},
	}

	pushCmd.Flags().StringVarP(&prometheusArg, "prometheus", "", prometheusArg, "Prometheus push gateway URL")
	rootCmd.AddCommand(pushCmd)
}
