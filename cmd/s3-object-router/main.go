package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	router "github.com/kayac/s3-object-router"
)

func main() {
	if err := _main(); err != nil {
		log.Println("[error]", err)
		os.Exit(1)
	}
}

func _main() error {
	var (
		bucket, keyPrefix, replacer       string
		timeKey, timeFormat               string
		gzip, timeParse, localTime, noPut bool
	)
	flag.StringVar(&bucket, "bucket", "", "destination S3 bucket name")
	flag.StringVar(&keyPrefix, "key-prefix", "", "prefix of S3 key")
	flag.BoolVar(&gzip, "gzip", true, "compress destination object by gzip")
	flag.StringVar(&replacer, "replacer", "", `wildcard string replacer JSON. e.g. {"foo.bar.*":"foo"}`)
	flag.BoolVar(&timeParse, "time-parse", false, "parse record value as time.Time with -time-format")
	flag.StringVar(&timeFormat, "time-format", time.RFC3339Nano, "format of time-parse")
	flag.StringVar(&timeKey, "time-key", router.DefaultTimeKey, "record key name for time-parse")
	flag.BoolVar(&localTime, "local-time", false, "set time zone to localtime for parsed time")
	flag.BoolVar(&noPut, "no-put", false, "do not put to s3")

	flag.Parse()

	opt := router.Option{
		Bucket:     bucket,
		KeyPrefix:  keyPrefix,
		Gzip:       gzip,
		Replacer:   replacer,
		TimeParse:  timeParse,
		TimeKey:    timeKey,
		TimeFormat: timeFormat,
		LocalTime:  localTime,
		PutS3:      !noPut,
	}
	sess, err := session.NewSession()
	if err != nil {
		return err
	}
	r, err := router.New(&opt, sess)
	if err != nil {
		return err
	}
	for _, s3url := range flag.Args() {
		if err := r.Run(context.Background(), s3url); err != nil {
			return err
		}
	}
	return nil
}
