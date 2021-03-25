package router_test

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

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

func TestCloudfrontRoute(t *testing.T) {
	opt := router.Option{
		Bucket:           "dummy",
		KeyPrefix:        `foo/{{ replace .x_edge_location }}/{{ .datetime.Format "2006-01-02" }}/`,
		Gzip:             false,
		Parser:           "cloudfront",
		TimeParse:        true,
		TimeKey:          "datetime",
		TimeFormat:       time.RFC3339,
		PutS3:            false,
		KeepOriginalName: false,
	}
	r, err := router.New(&opt)
	if err != nil {
		t.Error(err)
	}

	for _, src := range []io.Reader{
		bytes.NewReader(testCloudfrontSrcBytes),
	} {
		res, err := router.DoTestRoute(r, src, "s3://example-bucket/path/to/example-object")
		if err != nil {
			t.Error(err)
			continue
		}
		if len(res) != len(expectedCloudfrontRecords) {
			t.Errorf("unexpected routed records num")
			continue
		}
		// sha256sum of s3://example-bucket/path/to/example-object
		name := "f7ec2b7eb299d99468ff797fba836fa6cfc4389e21562f50a7d41ddcf43bfd01"
		for path, expected := range expectedCloudfrontRecords {
			u := path + name
			if expected != res[u] {
				t.Errorf("expected %s got %s", expected, res[u])
			}
		}
	}
}
