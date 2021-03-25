package router

import "github.com/pkg/errors"

//lineParser predefined errors
var (
	SkipLine = errors.New("Please skip this line.")
)

type lineParserFunc func([]byte, *record) error

func (p lineParserFunc) Parse(bs []byte, r *record) error {
	return p(bs, r)
}

type cloudfrontParser struct{}

func (p *cloudfrontParser) Parse(bs []byte, r *record) error {
	return errors.New("not implemented yet")
}
