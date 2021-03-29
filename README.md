# s3-object-router

S3 object router.

## Description

s3-object-router is a JSON object router for S3.

1. Read an object from S3 bucket.
1. Parse lines as JSON for each lines in the object.
1. A key prefix (expanded Go template with the JSON line) decides to destination object names.
1. Put objects to the routed destination which has the JSON lines.

## Install

### binary packages

[Releases](https://github.com/kayac/s3-object-router/releases).

### Homebrew tap

```console
$ brew install kayac/tap/s3-object-router
```

## Usage

### as CLI command

```console
$ s3-object-router \
    -bucket destination-bucket \
    -key-prefix 'path/to/{{ .tag }}' \
    s3://source-bucket/path/to/object
```

```
Usage of s3-object-router:
  -bucket string
    	destination S3 bucket name
  -format string
        convert the s3 object format. choices are json|none (default "none")
  -gzip
    	compress destination object by gzip (default true)
  -keep-original-name
    	keep original object base name
  -key-prefix string
    	prefix of S3 key
  -local-time
    	set time zone to localtime for parsed time
  -no-put
    	do not put to s3
  -parser string
        object record parser. choices are json|cloudfront (default "json")
  -replacer string
    	wildcard string replacer JSON. e.g. {"foo.bar.*":"foo"}
  -time-format string
    	format of time-parse (default "2006-01-02T15:04:05.999999999Z07:00")
  -time-key string
    	record key name for time-parse (default "time")
  -time-parse
    	parse record value as time.Time with -time-format
```

### as AWS Lambda function

`s3-object-router` binary also runs as AWS Lambda function called by S3 event trigger.

CLI options can be specified from environment variables. For example, when `BUCKET` environment variable is set, the value is set to `-bucket` option.

Example Lambda functions configuration.

```json
{
  "FunctionName": "s3-object-router",
  "Environment": {
    "Variables": {
      "BUCKET": "destination-bucket",
      "KEY_PREFIX": "{{ `/path/to/{{ .tag }}` }}"
    }
  },
  "Handler": "s3-object-router",
  "MemorySize": 128,
  "Role": "arn:aws:iam::0123456789012:role/lambda-function",
  "Runtime": "go1.x",
  "Timeout": 300
}
```

IAM Role of the function requires permissions (s3:GetObject and s3:PutObject) to source and destination objects.

### key-prefix

key-prefix renders Go template syntax with JSON objects.

For example,

- time-parse: true
- time-format: `2006-01-02T15:04:05` (See Go's time package https://golang.org/pkg/time/#Parse )
- key-prefix `path/to/{{ .tag }}/{{ .time.Format "2006-01-02-15" }}/{{ .source }}`
- Source S3 object
  ```json
  {"tag": "app.info", "time": "2020-08-24T11:22:33", "source": "stdout", "message": "xxx"}
  {"tag": "app.warn", "time": "2020-08-24T12:00:01", "source": "stderr", "message": "yyy"}
  ```

The first line will be routed to `path/to/app.info/2020-08-24-11/stdout/`, the second line will be routed to `path/to/app.warn/2020-08-24-12/stderr/`.

### replace function

`-replacer` defines a string replacer with wildcards. `replace` template function enables to replace strings.

For example,

- key-prefix: `path/to/{{ replace .tag }}`
- replacer:
    ```json
    {
        "app.warn*": "app.alert",
        "app.error*": "app.alert",
        "app.*": "app.normal"
    }
    ```
- Source S3 object
   ```json
   {"tag": "app.info.xxx", "message": "INFO msg"}
   {"tag": "app.error.yyy", "message": "ERROR msg"}
   {"tag": "app.warn.zzz", "message": "WARN msg"}
   ```

The first line will be routed to `path/to/app.normal/`, the second and third line will be routed to `path/to/app.alert`.

`-replacer` takes a definition as JSON string. The key defines matcher(may includes wildcard `*` and `?`) and the value defines replacement. The matchers works with an order that appears in JSON. When a matcher matches to a string, replace it to replacement and breaks (will not try other matchers).

### record parser

`-parser` specifies the Parser for the object record. In defualt, `json` is selected, and the S3 object parse as one JSON object for each record.

#### `cloudfront`

If "cloudfront" is selected, the S3 object will be parsed as CloudFront standard logs.  
(cf. https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/AccessLogs.html#AccessLogsFileNaming)

For example,

- time-parse: true
- time-key: `datetime`
- time-format: `2006-01-02T15:04:05Z` 
- key-prefix: `path/to/{{ .x_edge_location }}/{{ .datetime.Format "2006-01-02-15" }}`
- parser: `cloudfront`
- Source S3 object
   ```tsv
    #Version: 1.0
    #Fields: date time x-edge-location sc-bytes c-ip cs-method cs(Host) cs-uri-stem sc-status cs(Referer) cs(User-Agent) cs-uri-query cs(Cookie) x-edge-result-type x-edge-request-id x-host-header cs-protocol cs-bytes time-taken x-forwarded-for ssl-protocol ssl-cipher x-edge-response-result-type cs-protocol-version fle-status fle-encrypted-fields c-port time-to-first-byte x-edge-detailed-result-type sc-content-type sc-content-len sc-range-start sc-range-end
    2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	SOX4xwn4XV6Q4rgb7XiVGOHms_BGlTAC4KyHmureZmBNrjGdRLiNIQ==	d111111abcdef8.cloudfront.net	https	23	0.001	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.001	Hit	text/html	78	-	-
    2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	k6WGMNkEzR5BEM_SaF47gjtX9zBDO2m349OY2an0QPEaUum1ZOLrow==	d111111abcdef8.cloudfront.net	https	23	0.000	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.000	Hit	text/html	78	-	-
    2019-12-04	21:02:31	LAX1	392	192.0.2.100	GET	d111111abcdef8.cloudfront.net	/index.html	200	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Hit	f37nTMVvnKvV2ZSvEsivup_c2kZ7VXzYdjC-GUQZ5qNs-89BlWazbw==	d111111abcdef8.cloudfront.net	https	23	0.001	-	TLSv1.2	ECDHE-RSA-AES128-GCM-SHA256	Hit	HTTP/2.0	-	-	11040	0.001	Hit	text/html	78	-	-	
    2019-12-13	22:36:27	SEA19-C1	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/favicon.ico	502	http://www.example.com/	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Error	1pkpNfBQ39sYMnjjUQjmH2w1wdJnbHYTbag21o_3OfcQgPzdL2RSSQ==	www.example.com	http	675	0.102	-	-	-	Error	HTTP/1.1	-	-	25260	0.102	OriginDnsError	text/html	507	-	-
    2019-12-13	22:36:26	SEA19-C1	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/	502	-	Mozilla/5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/78.0.3904.108%20Safari/537.36	-	-	Error	3AqrZGCnF_g0-5KOvfA7c9XLcf4YGvMFSeFdIetR1N_2y8jSis8Zxg==	www.example.com	http	735	0.107	-	-	-	Error	HTTP/1.1	-	-	3802	0.107	OriginDnsError	text/html	507	-	-
    2019-12-13	22:37:02	SEA19-C2	900	192.0.2.200	GET	d111111abcdef8.cloudfront.net	/	502	-	curl/7.55.1	-	-	Error	kBkDzGnceVtWHqSCqBUqtA_cEs2T3tFUBbnBNkB9El_uVRhHgcZfcw==	www.example.com	http	387	0.103	-	-	-	Error	HTTP/1.1	-	-	12644	0.103	OriginDnsError	text/html	507	-	-
   ```

The 3rd line will be routed to `path/to/LAX1/2019-12-04-21/`, the 4th line will be routed to `path/to/SEA19-C1/2019-12-13-22/`.

`cloudfront` parser parses two header lines in S3 object.
At that time, the field name is converted according to the following rules.

  1. Replace `(` `)` `-` to `_`
  1. Replace to all lowercase
  1. Trim the right `_`

So can render any field in the key-prefix.; `cs(User-Agent)` can be rendered with `cs_user_agent`.


It also provides an RFC3399-formatted `datetime` field that combines the `date` and `time` fields of CloudFront's standard logs. Use with `-time-parse`,`-time-key`, `-time-format`.

If want to convert the routed S3 object format to JSON, please use `-format json`.
## LICENSE

MIT
