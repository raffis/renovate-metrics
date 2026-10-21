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
