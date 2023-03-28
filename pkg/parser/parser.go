package parser

import (
	"bufio"
	"encoding/json"
	"io"
)

type parser struct {
	r      io.Reader
	groups map[string]*collector
}

func NewParser(r io.Reader) *parser {
	return &parser{
		r:      r,
		groups: make(map[string]*collector),
	}
}

type Group struct {
	*collector
	Repository string
}

func (p *parser) Parse() (map[string]*collector, error) {
	scanner := bufio.NewScanner(p.r)

	var b []byte
	scanner.Buffer(b, 1000000)
	for scanner.Scan() {
		var line logLine
		err := json.Unmarshal(scanner.Bytes(), &line)
		if err == nil && line.Config != nil {
			group := p.group(line.Repository)
			group.Parse(line)

		}

		/*if err != nil {
			fmt.Printf("error: %#v -- %#v\n", line.Config, err)
		}*/
	}

	return p.groups, scanner.Err()
}

func (p *parser) group(repository string) *collector {
	if collector, has := p.groups[repository]; has {
		return collector
	}

	p.groups[repository] = NewCollector()
	return p.groups[repository]
}
