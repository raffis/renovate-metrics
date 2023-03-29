package parser

import (
	"bufio"
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
	scanner := bufio.NewScanner(p.r)

	var b []byte
	scanner.Buffer(b, p.opts.BufferSize)
	for scanner.Scan() {
		var line logLine
		err := json.Unmarshal(scanner.Bytes(), &line)
		if err == nil {
			if line.Repository == "" {
				continue
			}

			repository := p.repository(line.Repository)
			if err := repository.Parse(line); err != nil {
				p.opts.Logger.V(1).Info("failed to parse line", "error", err)
			}
		} else {
			p.opts.Logger.V(1).Info("failed to decode json line", "error", err)
		}
	}

	return p.repositories, scanner.Err()
}

func (p *parser) repository(repository string) *repository {
	if collector, has := p.repositories[repository]; has {
		return collector
	}

	p.repositories[repository] = NewRepository()
	return p.repositories[repository]
}
