package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"

	"github.com/go-logr/logr"
)

type parser struct {
	opts         ParserOptions
	r            io.Reader
	repositories map[string]*repository
}

type ParserOptions struct {
	BufferSize int
	Logger     logr.Logger
}

func NewParser(r io.Reader, opts ParserOptions) *parser {
	return &parser{
		opts:         opts,
		r:            r,
		repositories: make(map[string]*repository),
	}
}

type Repository struct {
	*repository
	Name string
}

func (p *parser) Parse() (map[string]*repository, error) {
	// Renovate can emit a single JSON log line larger than any fixed token size -- a
	// repository that matches hundreds of files dumps a multi-megabyte debug line. The
	// bufio.Scanner used here capped a line at BufferSize and returned bufio.ErrTooLong
	// once a line exceeded it; that error propagates through push's RunE and
	// rootCmd.Execute() to main(), which panic()s. The panic kills the process
	// mid-stream, and when renovate-metrics is piped from a live `renovate` process it
	// also closes the pipe and takes Renovate down with a broken pipe (EPIPE).
	//
	// bufio.Reader.ReadBytes has no such cap: it grows as needed and always drains to
	// EOF, so an oversized line is handled (or skipped) rather than being fatal, and the
	// stream is fully consumed either way.
	reader := bufio.NewReaderSize(p.r, p.opts.BufferSize)

	for {
		rawLine, readErr := reader.ReadBytes('\n')

		if len(rawLine) > 0 &&
			(bytes.Contains(rawLine, []byte(PackageFileUpdatesMessage)) ||
				bytes.Contains(rawLine, []byte(RepositoryFinishedMessage)) ||
				bytes.Contains(rawLine, []byte(BranchesInfoMessage))) {
			var line logLine
			err := json.Unmarshal(rawLine, &line)
			if err == nil {
				if line.Repository != "" {
					repository := p.repository(line.Repository)
					if err := repository.Parse(line); err != nil {
						p.opts.Logger.V(1).Info("failed to parse line", "error", err)
					}
				}
			} else {
				p.opts.Logger.V(1).Info("failed to decode json line", "error", err, "line", line)
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				return p.repositories, nil
			}

			return p.repositories, readErr
		}
	}
}

func (p *parser) repository(repository string) *repository {
	if collector, has := p.repositories[repository]; has {
		return collector
	}

	p.repositories[repository] = NewRepository(repository)
	return p.repositories[repository]
}
