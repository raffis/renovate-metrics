package parser

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/go-logr/logr"
	"github.com/raffis/renovate-metrics/pkg/backend"
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

func (p *parser) Parse(ctx context.Context, b backend.Backend) error {
	scanner := bufio.NewScanner(p.r)

	var buf []byte
	scanner.Buffer(buf, p.opts.BufferSize)

	for scanner.Scan() {
		rawLine := scanner.Bytes()
		if !bytes.Contains(rawLine, []byte(PackageFileUpdatesMessage)) &&
			!bytes.Contains(rawLine, []byte(RepositoryFinishedMessage)) &&
			!bytes.Contains(rawLine, []byte(BranchesInfoMessage)) {
			continue
		}

		var line logLine
		if err := json.Unmarshal(rawLine, &line); err != nil {
			p.opts.Logger.V(1).Info("failed to decode json line", "error", err)
			continue
		}
		if line.Repository == "" {
			continue
		}

		repo := p.repo(line.Repository)
		if err := repo.parse(line); err != nil {
			p.opts.Logger.V(1).Info("failed to parse line", "error", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	for _, repo := range p.repositories {
		if err := repo.flush(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) repo(name string) *repository {
	if r, has := p.repositories[name]; has {
		return r
	}
	r := newRepository(name)
	p.repositories[name] = r
	return r
}
