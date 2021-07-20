package main

import (
	"context"
	"flag"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	router "github.com/kayac/s3-object-router"
)

func main() {
	r, err := setup()
	if err != nil {
		log.Println("[error]", err)
		os.Exit(1)
	}

	if strings.HasPrefix(os.Getenv("AWS_EXECUTION_ENV"), "AWS_Lambda") ||
		os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		lambda.Start(lambdaHandler(r))
		return
	}
	if err := cli(r); err != nil {
		log.Println("[error]", err)
		os.Exit(1)
	}
}

func lambdaHandler(r *router.Router) func(context.Context, events.S3Event) error {
	return func(ctx context.Context, event events.S3Event) error {
		for _, record := range event.Records {
			u := url.URL{
				Scheme: "s3",
				Host:   record.S3.Bucket.Name,
				Path:   record.S3.Object.URLDecodedKey,
			}
			if err := r.Run(ctx, u.String()); err != nil {
				return err
			}
		}
		return nil
	}
}

func setup() (*router.Router, error) {
	var (
		bucket, keyPrefix, replacer, parser, objFromat string
		timeKey, timeFormat, timeZone                  string
		gzip, timeParse, localTime, noPut, keep        bool
	)
	flag.StringVar(&bucket, "bucket", "", "destination S3 bucket name")
	flag.StringVar(&keyPrefix, "key-prefix", "", "prefix of S3 key")
	flag.BoolVar(&gzip, "gzip", true, "compress destination object by gzip")
	flag.StringVar(&replacer, "replacer", "", `wildcard string replacer JSON. e.g. {"foo.bar.*":"foo"}`)
	flag.StringVar(&parser, "parser", "json", "object record parser. choices are json|cloudfront")
	flag.BoolVar(&timeParse, "time-parse", false, "parse record value as time.Time with -time-format")
	flag.StringVar(&timeFormat, "time-format", time.RFC3339Nano, "format of time-parse")
	flag.StringVar(&timeKey, "time-key", router.DefaultTimeKey, "record key name for time-parse")
	flag.BoolVar(&localTime, "local-time", false, "set time zone to localtime for parsed time")
	flag.StringVar(&timeZone, "time-zone", "", `set time zone to specified one for parsed time. e.g. "America/Los_Angeles" if use with -local-time,  -local-time takes precedence`)
	flag.BoolVar(&noPut, "no-put", false, "do not put to s3")
	flag.BoolVar(&keep, "keep-original-name", false, "keep original object base name")
	flag.StringVar(&objFromat, "format", "none", `convert the s3 object format. choices are json|none`)
	flag.VisitAll(envToFlag)
	flag.Parse()

	opt := router.Option{
		Bucket:           bucket,
		KeyPrefix:        keyPrefix,
		Gzip:             gzip,
		Replacer:         replacer,
		Parser:           parser,
		TimeParse:        timeParse,
		TimeKey:          timeKey,
		TimeFormat:       timeFormat,
		LocalTime:        localTime,
		TimeZone:         timeZone,
		PutS3:            !noPut,
		KeepOriginalName: keep,
		ObjectFormat:     objFromat,
	}
	log.Printf("[debug] option: %#v", opt)
	return router.New(&opt)
}

func cli(r *router.Router) error {
	for _, s3url := range flag.Args() {
		if err := r.Run(context.Background(), s3url); err != nil {
			return err
		}
	}
	return nil
}

func envToFlag(f *flag.Flag) {
	name := strings.ToUpper(strings.Replace(f.Name, "-", "_", -1))
	if s, ok := os.LookupEnv(name); ok {
		f.Value.Set(s)
	}
}
