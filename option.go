package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

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
	TimeZone         string `json:"timezone,omitempty"`
	Gzip             bool   `json:"gzip,omitempty"`
	Replacer         string `json:"replacer,omitempty"`
	Parser           string `json:"parser,omitempty"`
	PutS3            bool   `json:"put_s3,omitempty"`
	ObjectFormat     string `json:"object_format,omitempty"`
	KeepOriginalName bool   `json:"keep_original_name,omitempty"`

	replacer     replacer
	recordParser recordParser
	newEncoder   func() encoder
	newBuffer    func() buffer
	timeParser   timeParser
}

type replacer interface {
	Replace(string) string
}

type recordParser interface {
	Parse([]byte) ([]*record, error)
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
		opt.recordParser = recordParserFunc(func(b []byte) ([]*record, error) {
			r := newRecord(b)
			if err := json.Unmarshal(b, &(r.parsed)); err != nil {
				return nil, err
			}
			return []*record{r}, nil
		})
	case "json.Records":
		opt.recordParser = recordParserFunc(func(b []byte) ([]*record, error) {
			var m struct {
				Records []map[string]interface{} `json:"Records"`
			}
			if err := json.Unmarshal(b, &m); err != nil {
				return nil, err
			}
			records := make([]*record, 0, len(m.Records))
			for _, parsed := range m.Records {
				p := parsed
				records = append(records, &record{parsed: p, raw: nil})
			}
			return records, nil
		})
	case "cloudfront":
		opt.recordParser = &cloudfrontParser{}
	default:
		return errors.New("parser must be string any of json|cloudfront")
	}
	if opt.TimeParse {
		p := timeParser{layout: opt.TimeFormat}
		switch {
		case opt.LocalTime:
			p.loc = time.Local
		case opt.TimeZone != "":
			var err error
			p.loc, err = time.LoadLocation(opt.TimeZone)
			if err != nil {
				return fmt.Errorf("timezone is invalid, %w", err)
			}
		default:
			p.loc = time.UTC
		}
		opt.timeParser = p
	}
	if opt.TimeKey == "" {
		opt.TimeKey = DefaultTimeKey
	}

	if opt.Gzip {
		opt.newBuffer = newGzipBuffer
	} else {
		opt.newBuffer = func() buffer {
			return new(bytes.Buffer)
		}
	}

	switch opt.ObjectFormat {
	case "", "none":
		if opt.Parser == "json.Records" {
			return errors.New("parser must not be json.Records when object-format is none")
		}
		opt.newEncoder = func() encoder {
			return newNoneEncoder(opt.newBuffer())
		}
	case "json":
		opt.newEncoder = func() encoder {
			return newJSONEncoder(opt.newBuffer())
		}
	default:
		return errors.New("format must be string any of json|none")
	}
	return nil
}
