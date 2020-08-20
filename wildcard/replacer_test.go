package wildcard

import (
	"testing"
)

var testReplacer = []string{
	"nginx.stdout*", "nginx.access",
	"nginx.stderr*", "nginx.error",
	"app.warn.**", "app.warn",
	"app.*", "app",
}

var testReplaceSet = [][]string{
	{"app.warn.xxx", "app.warn"},
	{"app.info.foo", "app"},
	{"nginx.stdout.xxx", "nginx.access"},
	{"nginx.stderr.yyy", "nginx.error"},
	{"prefix.app.test.xxx", "prefix.app.test.xxx"},
}

func TestReplacer(t *testing.T) {
	rp := NewReplacer(testReplacer...)
	for _, ts := range testReplaceSet {
		if rs := rp.Replace(ts[0]); rs != ts[1] {
			t.Errorf("unexpected replace %s -> %s (expected %s)", ts[0], rs, ts[1])
		}
	}
}
