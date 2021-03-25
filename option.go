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
	Bucket           string `json:"bucket,omitempty"`
	KeyPrefix        string `json:"key_prefix,omitempty"`
	TimeParse        bool   `json:"time_parse,omitempty"`
	TimeKey          string `json:"time_key,omitempty"`
	TimeFormat       string `json:"time_format,omitempty"`
	LocalTime        bool   `json:"local_time,omitempty"`
	Gzip             bool   `json:"gzip,omitempty"`
	Replacer         string `json:"replacer,omitempty"`
	Parser           string `json:"parser,omitempty"`
	PutS3            bool   `json:"put_s3,omitempty"`
	KeepOriginalLine bool   `json:"keep_original_line,omitempty"`
	KeepOriginalName bool   `json:"keep_original_name,omitempty"`

	replacer   replacer
	lineParser lineParser
	timeParser timeParser
}

type replacer interface {
	Replace(string) string
}

type lineParser interface {
	Parse([]byte, *record) error
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
	switch opt.Parser {
	case "", "json":
		opt.lineParser = lineParserFunc(func(b []byte, r *record) error {
			return json.Unmarshal(b, r)
		})
	case "cloudfront":
		opt.lineParser = &cloudfrontParser{}
	default:
		return errors.New("parser must be string any of json|cloudfront")
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
