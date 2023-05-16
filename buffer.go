package router

import (
	"bytes"
	"compress/gzip"
)

type buffer interface {
	Write([]byte) (int, error)
	Bytes() []byte
}

type gzBuffer struct {
	bytes.Buffer
	gz *gzip.Writer
}

func newGzipBuffer() buffer {
	buf := &gzBuffer{}
	buf.gz = gzip.NewWriter(&buf.Buffer)
	return buf
}

func (buf *gzBuffer) Write(p []byte) (int, error) {
	return buf.gz.Write(p)
}

func (buf *gzBuffer) Close() error {
	return buf.gz.Close()
}
