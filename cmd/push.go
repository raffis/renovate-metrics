package main

import (
	"log"
	"os"

	"github.com/go-logr/stdr"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/raffis/renovate-metrics/pkg/parser"
	"github.com/spf13/cobra"
)

var (
	prometheusArg = "http://localhost:9091"
	bufferSize    = 10485760
	logLevel      = 0
)

func newStdLogger(flags int) stdr.StdLogger {
	return log.New(os.Stdout, "", flags)
}

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

				defer func() { _ = f.Close() }()
				file = f
			}

			log := stdr.New(newStdLogger(log.Lshortfile))
			stdr.SetVerbosity(logLevel)

			parser := parser.NewParser(file, parser.ParserOptions{
				BufferSize: bufferSize,
				Logger:     log,
			})

			collectors, err := parser.Parse()
			if err != nil {
				return err
			}

			client := push.New(prometheusArg, "renovate")

			for repository, collector := range collectors {
				client.Grouping("r", repository)

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
	pushCmd.Flags().IntVarP(&bufferSize, "buffer-size", "", bufferSize, "Buffer size while parsing input")
	pushCmd.Flags().IntVarP(&logLevel, "log-level", "", logLevel, "Log Level (Default is 0 which is no logging)")
	rootCmd.AddCommand(pushCmd)
}
