package router

import (
	"encoding/json"
	"fmt"
)

// LF represents LineFeed \n
var LF = []byte("\n")

type encoder interface {
	Encode(*record) error
	Buffer() buffer
}

type noneEncoder struct {
	body buffer
}

func newNoneEncoder(body buffer) encoder {
	return &noneEncoder{
		body: body,
	}
}

func (e *noneEncoder) Encode(rec *record) error {
	if _, err := e.body.Write(rec.raw); err != nil {
		return err
	}
	_, err := e.body.Write(LF)
	return err
}

func (e *noneEncoder) Buffer() buffer {
	return e.body
}

type jsonEncoder struct {
	body buffer
}

func newJSONEncoder(body buffer) encoder {
	return &jsonEncoder{
		body: body,
	}
}

func (e *jsonEncoder) Encode(rec *record) error {
	bytes, err := json.Marshal(rec.parsed)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	if _, err := e.body.Write(bytes); err != nil {
		return err
	}
	_, err = e.body.Write(LF)
	return err
}

func (e *jsonEncoder) Buffer() buffer {
	return e.body
}
