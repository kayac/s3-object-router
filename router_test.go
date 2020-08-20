package router_test

import (
	"bytes"
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

var testSrc = bytes.NewBufferString(strings.Join(testRecords, "\n"))

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
	res := router.DoTestRoute(r, testSrc, "example")
	if len(res) != len(expectedRecords) {
		t.Errorf("unexpected routed records num")
	}
	for u, expected := range expectedRecords {
		if expected != res[u] {
			t.Errorf("expected %s got %s", expected, res[u])
		}
	}
}
