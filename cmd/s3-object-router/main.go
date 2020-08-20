package main

import (
	"context"
	"flag"
	"log"
	"os"

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
		bucket, keyPrefix, replacer string
		gzip                        bool
	)
	flag.StringVar(&bucket, "bucket", "", "destination S3 bucket name")
	flag.StringVar(&keyPrefix, "key-prefix", "", "prefix of S3 key")
	flag.BoolVar(&gzip, "gzip", true, "compress destination object by gzip")
	flag.StringVar(&replacer, "replacer", "", `wildcard string replacer JSON. e.g. {"foo.bar.*":"foo"}`)
	flag.Parse()

	opt := router.Option{
		Bucket:    bucket,
		KeyPrefix: keyPrefix,
		Gzip:      gzip,
		Replacer:  replacer,
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
