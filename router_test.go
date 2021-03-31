package router_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"mime/multipart"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	router "github.com/kayac/s3-object-router"
)

var updateFlag = flag.Bool("update", false, "update golden files")

func TestMain(t *testing.T) {
	flag.Parse()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(testSrcBytes)
	w.Close()
	testGzippedSrcBytes = b.Bytes()
}

type testParserConfig struct {
	router.Option
	Sources        []string `json:"sources"`
	EnableGzipTest bool     `json:"enable_gzip_test"`
}

func TestParser(t *testing.T) {
	cases, err := ioutil.ReadDir("testdata")
	if err != nil {
		t.Logf("can not read testdata:%s", err)
		t.FailNow()
	}
	for _, c := range cases {
		if !c.IsDir() {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			testParser(t, c.Name())
		})
	}
}

func testParser(t *testing.T, caseDirName string) {
	fp, err := os.Open(filepath.Join("testdata", caseDirName, "config.json"))
	if err != nil {
		t.Logf("can not open test config:%s", err)
		t.FailNow()
	}
	defer fp.Close()
	decoder := json.NewDecoder(fp)
	var config testParserConfig
	if err := decoder.Decode(&config); err != nil {
		t.Logf("can not parse test config:%s", err)
		t.FailNow()
	}

	r, err := router.New(&config.Option)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	sfps := make(map[string]io.ReadCloser, len(config.Sources))
	defer func() {
		for _, sfp := range sfps {
			sfp.Close()
		}
	}()
	for _, src := range config.Sources {
		path := filepath.Join("testdata", src)
		basename := filepath.Base(path)
		sfp, err := os.Open(path)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		if config.EnableGzipTest {
			var raw, gzipped bytes.Buffer
			gw := gzip.NewWriter(&gzipped)
			w := io.MultiWriter(&raw, gw)
			io.Copy(w, sfp)
			sfp.Close()
			gw.Close()
			sfps[basename] = io.NopCloser(&raw)
			sfps[basename+"_gzipped"] = io.NopCloser(&gzipped)
		} else {
			sfps[basename] = sfp
		}
	}
	for name, sfp := range sfps {
		res, err := router.DoTestRoute(r, sfp, "s3://example-bucket/path/to/example-object")
		if err != nil {
			t.Error(err)
			continue
		}
		goldenFile := filepath.Join("testdata", caseDirName, name+".golden")
		if *updateFlag {
			writeParserGolden(t, goldenFile, res)
		}
		expected := readParserGolden(t, goldenFile)
		if !reflect.DeepEqual(expected, res) {
			t.Error("unexpected routed data")
			for u, expectedContent := range expected {
				if expectedContent != res[u] {
					t.Errorf("%s:expected %s got %s", u, expectedContent, res[u])
				}
			}
		}
	}
}

const (
	parserGoldenBoundary = "----s3-object-router-parser-test----"
)

func writeParserGolden(t *testing.T, goldenFile string, res map[string]string) {
	t.Helper()
	fp, err := os.OpenFile(
		goldenFile,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0644,
	)
	if err != nil {
		t.Logf("can not create golden file: %s", err)
		t.FailNow()
	}
	defer fp.Close()
	w := multipart.NewWriter(fp)
	w.SetBoundary(parserGoldenBoundary)
	for dest, content := range res {
		if err := w.WriteField(dest, content); err != nil {
			t.Logf("can not write golden data: %s", err)
			t.FailNow()
		}
	}
	w.Close()
}

func readParserGolden(t *testing.T, goldenFile string) map[string]string {
	t.Helper()
	fp, err := os.Open(goldenFile)
	res := map[string]string{}
	if err != nil {
		t.Logf("can not open golden file: %s", err)
		t.FailNow()
	}
	defer fp.Close()
	r := multipart.NewReader(fp, parserGoldenBoundary)
	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("can not get golden next part: %s", err)
			t.FailNow()
		}
		content, err := ioutil.ReadAll(part)
		if err != nil {
			t.Logf("can not read golden part: %s", err)
			t.FailNow()
		}
		res[part.FormName()] = string(content)
		part.Close()
	}
	return res
}
