package router

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

const (
	defaultCloudFrontNumColumns = 33
)

//lineParser predefined errors
var (
	SkipLine = errors.New("Please skip this line.")
)

type lineParserFunc func([]byte, *record) error

func (p lineParserFunc) Parse(bs []byte, r *record) error {
	return p(bs, r)
}

type cloudfrontParser struct {
	version string
	fields  []string
}

func (p *cloudfrontParser) Parse(bs []byte, r *record) error {
	str := string(bs)
	rec := make(record, defaultCloudFrontNumColumns)
	*r = rec
	if str[0] == '#' {
		part := strings.SplitN(str[1:], ":", 2)
		if len(part) != 2 {
			return SkipLine
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
				fields = append(fields, field)
			}
			p.fields = fields
		}
		return SkipLine
	}
	values := strings.Split(str, "\t")
	if len(values) > len(p.fields) {
		return fmt.Errorf("this row has more values ​​than fields, num of values = %d, num of feilds = %d", len(values), len(p.fields))
	}
	var dateValue, timeValue string
	for i, field := range p.fields {
		rec[field] = values[i]
		if field == "date" {
			dateValue = values[i]
		}
		if field == "time" {
			timeValue = values[i]
		}
	}
	rec["datetime"] = dateValue + "T" + timeValue + "Z"
	return nil
}
