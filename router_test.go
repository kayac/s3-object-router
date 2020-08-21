package router_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
	"time"

	router "github.com/kayac/s3-object-router"
)

var testRecords = []string{
	`{"tag":"app.info","message":"[INFO] app","time":"2020-08-20T15:42:02+09:00"}`,
	`{"tag":"app.error","message":"[ERROR] app","time":"2020-08-20T16:42:02+09:00"}`,
	`{"tag":"batch.info","message":"[INFO] batch","time":"2020-08-19T15:42:02+09:00"}`,
	`{"tag":"batch.warn","message":"[WARN] batch","time":"2020-08-20T15:43:11+09:00"}`,
	`{"tag":"app.warn","message":"[WARN] app","time":"2020-08-20T15:43:11+09:00"}`,
	`{"tag":"app.warn","message":"[WARN] app","time":"2020-08-21T15:43:11+09:00"}`,
}

var testSrcBytes = []byte(strings.Join(testRecords, "\n"))
var testSrc = bytes.NewBuffer(testSrcBytes)
var testGzippedSrc = new(bytes.Buffer)

var expectedRecords = map[string]string{
	"s3://dummy/foo/app/2020-08-20/example":        concat(testRecords[0], testRecords[1], testRecords[4]),
	"s3://dummy/foo/app/2020-08-21/example":        concat(testRecords[5]),
	"s3://dummy/foo/batch.info/2020-08-19/example": concat(testRecords[2]),
	"s3://dummy/foo/batch.warn/2020-08-20/example": concat(testRecords[3]),
}

func concat(strs ...string) string {
	var b strings.Builder
	for _, s := range strs {
		b.WriteString(s)
		b.WriteString("\n")
	}
	return b.String()
}

func TestMain(t *testing.T) {
	w := gzip.NewWriter(testGzippedSrc)
	w.Write(testSrcBytes)
	w.Close()
}

func TestRoute(t *testing.T) {
	opt := router.Option{
		Bucket:     "dummy",
		KeyPrefix:  `foo/{{ replace .tag }}/{{ .time.Format "2006-01-02" }}/`,
		Gzip:       false,
		Replacer:   `{"app.*":"app"}`,
		TimeParse:  true,
		TimeFormat: time.RFC3339,
		PutS3:      false,
	}
	r, err := router.New(&opt)
	if err != nil {
		t.Error(err)
	}

	for _, src := range []io.Reader{testSrc, testGzippedSrc} {
		res, err := router.DoTestRoute(r, src, "example")
		if err != nil {
			t.Error(err)
			continue
		}
		if len(res) != len(expectedRecords) {
			t.Errorf("unexpected routed records num")
			continue
		}
		for u, expected := range expectedRecords {
			if expected != res[u] {
				t.Errorf("expected %s got %s", expected, res[u])
			}
		}
	}
}
