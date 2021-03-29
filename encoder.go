package router

import (
	"encoding/json"
	"log"
)

// LF represents LineFeed \n
var LF = []byte("\n")

type noneEncoder struct {
	body buffer
}

func newNoneEncoder(body buffer) encoder {
	return &noneEncoder{
		body: body,
	}
}

func (e *noneEncoder) Encode(_ record, recordBytes []byte) error {
	if _, err := e.body.Write(recordBytes); err != nil {
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

func (e *jsonEncoder) Encode(rec record, _ []byte) error {

	bytes, err := json.Marshal(rec)
	if err != nil {
		log.Println("[warn] failed to generate json record", err)
		return err
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
