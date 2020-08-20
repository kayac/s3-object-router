package router

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kayac/s3-object-router/wildcard"
	"github.com/mickep76/mapslice-json"
	"github.com/pkg/errors"
)

var DefaultTimeKey = "time"

// Option represents option values of router
type Option struct {
	Bucket     string
	KeyPrefix  string
	TimeParse  bool
	TimeKey    string
	TimeFormat string
	LocalTime  bool
	Gzip       bool
	Replacer   string
	PutS3      bool

	replacer   replacer
	timeParser timeParser
}

type replacer interface {
	Replace(string) string
}

type timeParser struct {
	layout string
	loc    *time.Location
}

func (p timeParser) Parse(s string) (time.Time, error) {
	t, err := time.Parse(p.layout, s)
	if err != nil {
		return t, err
	}
	return t.In(p.loc), nil
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
	if opt.TimeParse {
		p := timeParser{layout: opt.TimeFormat}
		if opt.LocalTime {
			p.loc = time.Local
		} else {
			p.loc = time.UTC
		}
		opt.timeParser = p
	}
	if opt.TimeKey == "" {
		opt.TimeKey = DefaultTimeKey
	}
	return nil
}
