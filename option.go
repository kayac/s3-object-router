package router

import (
	"encoding/json"
	"strings"

	"github.com/kayac/s3-object-router/wildcard"
	"github.com/mickep76/mapslice-json"
	"github.com/pkg/errors"
)

// Option represents option values of router
type Option struct {
	Bucket     string
	KeyPrefix  string
	TimeParse  bool
	TimeKey    string
	TimeFormat string
	Gzip       bool
	Replacer   string

	replacer replacer
}

type replacer interface {
	Replace(string) string
}

// Init initializes option struct.
func (opt *Option) Init() error {
	if opt.Bucket == "" {
		return errors.New("bucket must not be empty")
	}
	if opt.KeyPrefix == "" {
		return errors.New("key-prefix must not be empty")
	}
	if opt.Replacer != "" {
		mp := mapslice.MapSlice{}
		if err := json.Unmarshal([]byte(opt.Replacer), &mp); err != nil {
			return errors.Wrap(err, "invalid replacer")
		}
		args := make([]string, 0, len(mp)*2)
		for _, kv := range mp {
			if key, ok := kv.Key.(string); !ok {
				return errors.New("replacer pattern must be string")
			} else {
				args = append(args, key)
			}
			if value, ok := kv.Value.(string); !ok {
				return errors.New("replacer replacement must be string")
			} else {
				args = append(args, value)
			}
		}
		opt.replacer = wildcard.NewReplacer(args...)
	} else {
		opt.replacer = strings.NewReplacer() // nop replacer
	}
	return nil
}
