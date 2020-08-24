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
  -gzip
    	compress destination object by gzip (default true)
  -key-prefix string
    	prefix of S3 key
  -local-time
    	set time zone to localtime for parsed time
  -no-put
    	do not put to s3
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

The first line will be routed to `path/to/app.info/2020-08-24-11/stdout/`, the second line will be routed to `path/to/app.info/2020-08-24-12/stderr/`.

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

## LICENSE

MIT
