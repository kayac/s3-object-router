package router

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

const (
	defaultCloudFrontNumColumns = 33
)

// recordParser predefined errors
var (
	SkipLine = errors.New("Please skip this line.")
)

type recordParserFunc func([]byte) (*record, error)

func (p recordParserFunc) Parse(bs []byte) (*record, error) {
	return p(bs)
}

type cloudfrontParser struct {
	version string
	fields  []string
}

func (p *cloudfrontParser) Parse(bs []byte) (*record, error) {
	str := string(bs)
	rec := newRecord(bs)
	if str[0] == '#' {
		part := strings.SplitN(str[1:], ":", 2)
		if len(part) != 2 {
			return nil, SkipLine
		}
		key := strings.TrimSpace(part[0])
		value := strings.TrimSpace(part[1])
		switch key {
		case "Version":
			p.version = value
		case "Fields":
			rawFields := strings.Split(value, " ")
			//convert to snake case
			fields := make([]string, 0, len(rawFields))
			replaceTargets := []string{"(", ")", "-"}
			for _, rawField := range rawFields {
				field := rawField
				for _, target := range replaceTargets {
					field = strings.ReplaceAll(field, target, "_")
				}
				field = strings.ToLower(field)
				field = strings.TrimRight(field, "_")
				fields = append(fields, field)
			}
			p.fields = fields
		}
		return nil, SkipLine
	}
	values := strings.Split(str, "\t")
	if len(values) > len(p.fields) {
		return nil, fmt.Errorf("this row has more values ​​than fields, num of values = %d, num of feilds = %d", len(values), len(p.fields))
	}
	var dateValue, timeValue string
	for i, field := range p.fields {
		rec.parsed[field] = values[i]
		if field == "date" {
			dateValue = values[i]
		}
		if field == "time" {
			timeValue = values[i]
		}
	}
	rec.parsed["datetime"] = dateValue + "T" + timeValue + "Z"
	return rec, nil
}
