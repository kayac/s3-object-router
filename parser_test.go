package router_test

import (
	"encoding/json"
	"io"
	"io/fs"
	"io/ioutil"
	"mime/multipart"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	router "github.com/kayac/s3-object-router"
)

//from https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/AccessLogs.html#AccessLogsFileNaming
var testCloudfrontRecords = []string{
	`#Version: 1.0`,
	`#Fields: date time x-edge-location sc-bytes c-ip cs-method cs(Host) cs-uri-stem sc-status cs(Referer) cs(User-Agent) cs-uri-query cs(Cookie) x-edge-result-type x-edge-request-id x-host-header cs-protocol cs-bytes time-taken x-forwarded-for ssl-protocol ssl-cipher x-edge-response-result-type cs-protocol-version fle-status fle-encrypted-fields c-port time-to-first-byte x-edge-detailed-result-type sc-content-type sc-content-len sc-range-start sc-range-end`,
	`2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	SOX4xwn4XV6Q4rgb7XiVGOHms_BGlTAC4KyHmureZmBNrjGdRLiNIQ==	d111111abcdef8.cloudfront.net	https	23	0.001	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.001	Hit	text/html	78	-	-`,
	`2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	k6WGMNkEzR5BEM_SaF47gjtX9zBDO2m349OY2an0QPEaUum1ZOLrow==	d111111abcdef8.cloudfront.net	https	23	0.000	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.000	Hit	text/html	78	-	-`,
	`2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	f37nTMVvnKvV2ZSvEsivup_c2kZ7VXzYdjC-GUQZ5qNs-89BlWazbw==	d111111abcdef8.cloudfront.net	https	23	0.001	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.001	Hit	text/html	78	-	-`,
	`2019-12-13	22:36:27	SEA19-C1	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/favicon.ico	502	http://www.example.com/	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Error	1pkpNfBQ39sYMnjjUQjmH2w1wdJnbHYTbag21o_3OfcQgPzdL2RSSQ==	www.example.com	http	675	0.102	-	-	-	Error	HTTP/1.1	-	-	25260	0.102	OriginDnsError	text/html	507	-	-`,
	`2019-12-13	22:36:26	SEA19-C1	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/	502	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Error	3AqrZGCnF_g0-5KOvfA7c9XLcf4YGvMFSeFdIetR1N_2y8jSis8Zxg==	www.example.com	http	735	0.107	-	-	-	Error	HTTP/1.1	-	-	3802	0.107	OriginDnsError	text/html	507	-	-`,
	`2019-12-13	22:37:02	SEA19-C2	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/	502	-	curl/7.55.1	-	-	Error	kBkDzGnceVtWHqSCqBUqtA_cEs2T3tFUBbnBNkB9El_uVRhHgcZfcw==	www.example.com	http	387	0.103	-	-	-	Error	HTTP/1.1	-	-	12644	0.103	OriginDnsError	text/html	507	-	-`,
}

var testCloudfrontSrcBytes = []byte(strings.Join(testCloudfrontRecords, "\n"))

var expectedCloudfrontRecords = map[string]string{
	"s3://dummy/foo/LAX1/2019-12-04/":     concat(testCloudfrontRecords[2], testCloudfrontRecords[3], testCloudfrontRecords[4]),
	"s3://dummy/foo/SEA19-C1/2019-12-13/": concat(testCloudfrontRecords[5], testCloudfrontRecords[6]),
	"s3://dummy/foo/SEA19-C2/2019-12-13/": concat(testCloudfrontRecords[7]),
}

type testParserConfig struct {
	router.Option
	Sources []string `json:"sources"`
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
			testParser(t, c)
		})
	}
}

func testParser(t *testing.T, caseDir fs.FileInfo) {
	fp, err := os.Open(filepath.Join("testdata", caseDir.Name(), "config.json"))
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
		path := filepath.Join("testdata", caseDir.Name(), src)
		sfp, err := os.Open(path)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		sfps[path] = sfp
	}
	for path, sfp := range sfps {
		res, err := router.DoTestRoute(r, sfp, "s3://example-bucket/path/to/example-object")
		if err != nil {
			t.Error(err)
			continue
		}
		goldenFile := path + ".golden"
		if *updateFlag {
			writeParserGolden(t, goldenFile, res)
		}
		expected := readParserGolden(t, goldenFile)
		if !reflect.DeepEqual(expected, res) {
			t.Error("unexpected routed data")
			for u, expectedContent := range expected {
				if expectedContent != res[u] {
					t.Errorf("expected %s got %s", expectedContent, res[u])
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
