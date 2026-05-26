package main

import (
	"log"
	"os"

	"github.com/go-logr/stdr"
	"github.com/raffis/renovate-metrics/pkg/backend"
	"github.com/raffis/renovate-metrics/pkg/parser"
	"github.com/spf13/cobra"
)

var (
	prometheusArg    = "http://localhost:9091"
	job              = "renovate"
	bufferSize       = 10485760
	logLevel         = 0
	otlpEndpoint     = ""
	otlpGRPCEndpoint = ""
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

			var b backend.Backend
			switch {
			case otlpEndpoint != "":
				be, err := backend.NewOTELBackend(cmd.Context(), otlpEndpoint, "http")
				if err != nil {
					return err
				}
				b = be
			case otlpGRPCEndpoint != "":
				be, err := backend.NewOTELBackend(cmd.Context(), otlpGRPCEndpoint, "grpc")
				if err != nil {
					return err
				}
				b = be
			default:
				b = backend.NewPrometheusBackend(prometheusArg, job)
			}
			defer func() { _ = b.Shutdown(cmd.Context()) }()

			p := parser.NewParser(file, parser.ParserOptions{
				BufferSize: bufferSize,
				Logger:     log,
			})

			if err := p.Parse(cmd.Context(), b); err != nil {
				return err
			}

			return b.Flush(cmd.Context())
		},
	}

	pushCmd.Flags().StringVarP(&prometheusArg, "prometheus", "", prometheusArg, "Prometheus push gateway URL")
	pushCmd.Flags().StringVarP(&job, "job", "", job, "Value of job label used when pushing metrics")
	pushCmd.Flags().IntVarP(&bufferSize, "buffer-size", "", bufferSize, "Buffer size while parsing input")
	pushCmd.Flags().IntVarP(&logLevel, "log-level", "", logLevel, "Log Level (Default is 0 which is no logging)")
	pushCmd.Flags().StringVar(&otlpEndpoint, "otlp-endpoint", otlpEndpoint, "OTLP HTTP endpoint URL (e.g. http://otelcol:4318)")
	pushCmd.Flags().StringVar(&otlpGRPCEndpoint, "otlp-grpc-endpoint", otlpGRPCEndpoint, "OTLP gRPC endpoint (e.g. otelcol:4317)")
	rootCmd.AddCommand(pushCmd)
}
