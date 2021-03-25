package router

import "github.com/pkg/errors"

type lineParserFunc func([]byte, *record) error

func (p lineParserFunc) Parse(bs []byte, r *record) error {
	return p(bs, r)
}

type cloudfrontParser struct{}

func (p *cloudfrontParser) Parse(bs []byte, r *record) error {
	return errors.New("not implemented yet")
}
