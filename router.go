package router

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// MaxConcurrency represents maximum concurrency for uploading to S3
var MaxConcurrency = 10

// MetaHeaderName is metadata name to set routed objects.
var MetaHeaderName = "x-amz-meta-route-original"

var gzipMagicBytes = []byte{0x1f, 0x8b}

var gzipSuffix = ".gz"

var (
	initialBufSize = 64 * 1024
	maxBufSize     = initialBufSize * 10
)

// Router represents s3-object-router application
type Router struct {
	awsConf aws.Config

	// s3 clients for each region
	s3 map[string]*s3.Client
	s3Lock sync.Mutex

	// s3 bucket region cache
	s3bucketRegion map[string]string
	s3bucketRegionLock sync.Mutex

	option *Option
	sem    *semaphore.Weighted

	genKeyPrefix func(*record) (string, error)
}

// New creates a new router
func New(opt *Option) (*Router, error) {
	if err := opt.Init(); err != nil {
		return nil, err
	}

	awsConf, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, err
	}

	tmpl := template.New("prefixGenerator")
	tmpl.Funcs(template.FuncMap{
		"replace": opt.replacer.Replace,
	})
	if tmpl, err = tmpl.Parse(opt.KeyPrefix); err != nil {
		return nil, err
	}

	return &Router{
		awsConf: awsConf,
		s3: map[string]*s3.Client{
			awsConf.Region: s3.NewFromConfig(awsConf),
		},
		s3bucketRegion: map[string]string{},
		option:         opt,
		sem:            semaphore.NewWeighted(int64(MaxConcurrency)),
		genKeyPrefix: func(r *record) (string, error) {
			var b strings.Builder
			if err := tmpl.Execute(&b, r.parsed); err != nil {
				return "", err
			}
			return b.String(), nil
		},
	}, nil
}

// Run runs router
func (r *Router) Run(ctx context.Context, s3url string) error {
	log.Println("[info] run", s3url)
	src, err := r.getS3Object(ctx, s3url)
	if err != nil {
		return err
	}
	defer src.Close()
	keyBase := r.genKeyBase(s3url)
	meta := map[string]string{
		MetaHeaderName: s3url,
	}
	return r.Route(ctx, src, keyBase, meta)
}

func (r *Router) Route(ctx context.Context, src io.Reader, keyBase string, meta map[string]string) error {
	dests, err := r.route(src, keyBase)
	if err != nil {
		return err
	}

	eg := errgroup.Group{}
	for dest, buf := range dests {
		dest, buf := dest, buf
		if c, isCloser := buf.(io.Closer); isCloser {
			c.Close()
		}
		body := bytes.NewReader(buf.Bytes())
		log.Println("[info] route", dest.String(), body.Len(), "bytes")
		if r.option.PutS3 {
			eg.Go(func() error {
				return r.putToS3(ctx, dest, body, meta)
			})
		}
	}

	return eg.Wait()
}

func unGzip(src io.Reader) (io.Reader, error) {
	bufSrc := bufio.NewReader(src)
	if b, err := bufSrc.Peek(2); err != nil {
		// less than 2 bytes. returns original
		return bufSrc, nil
	} else if bytes.Equal(b, gzipMagicBytes) {
		// gzipped
		return gzip.NewReader(bufSrc)
	} else {
		// raw
		return bufSrc, nil
	}
}

func (r *Router) genKeyBase(s3url string) string {
	if r.option.KeepOriginalName {
		return path.Base(s3url)
	}
	sum := sha256.Sum256([]byte(s3url))
	return fmt.Sprintf("%x", sum)
}

func (r *Router) route(src io.Reader, keyBase string) (map[destination]buffer, error) {
	src, err := unGzip(src)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(src)
	recordParser := r.option.recordParser
	buf := make([]byte, initialBufSize)
	scanner.Buffer(buf, maxBufSize)

	encs := make(map[destination]encoder)

	for scanner.Scan() {
		recordBytes := scanner.Bytes()
		recs, err := recordParser.Parse(recordBytes)
		if err != nil {
			if err != SkipLine {
				log.Println("[warn] failed to parse record", err)
			}
			continue
		}
		if len(recs) == 0 {
			continue
		}
	RECORD:
		for _, rec := range recs {
			if r.option.TimeParse {
				if ts, ok := rec.parsed[r.option.TimeKey].(string); ok {
					rec.parsed[r.option.TimeKey], err = r.option.timeParser.Parse(ts)
					if err != nil {
						log.Println("[warn] failed to parse time", err)
					}
				}
			}
			d, err := r.genDestination(rec, keyBase)
			if err != nil {
				log.Println("[warn] failed to generate destination", err)
				continue RECORD
			}
			enc := encs[d]
			if enc == nil {
				enc = r.option.newEncoder()
			}
			if err := enc.Encode(rec); err != nil {
				log.Printf("[warn] failed to encode record %s: %#v\n", err, rec)
				continue RECORD
			}
			encs[d] = enc
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	dests := make(map[destination]buffer, len(encs))
	for d, enc := range encs {
		dests[d] = enc.Buffer()
	}
	return dests, nil
}

func (r *Router) getS3Object(ctx context.Context, s3url string) (io.ReadCloser, error) {
	u, err := url.Parse(s3url)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "s3" {
		return nil, errors.New("s3:// required")
	}

	s3c, err := r.s3Client(ctx, u.Host)
	if err != nil {
		return nil, err
	}

	out, err := s3c.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(u.Host),
		Key:    aws.String(strings.TrimPrefix(u.Path, "/")),
	})
	if err != nil {
		return nil, err
	}
	// loop guard
	for name, value := range out.Metadata {
		if strings.ToLower(name) == MetaHeaderName {
			out.Body.Close()
			return nil, fmt.Errorf("%s seems to be an already routed object. original: %s", s3url, value)
		}
	}

	return out.Body, nil
}

func (r *Router) genDestination(rec *record, name string) (destination, error) {
	prefix, err := r.genKeyPrefix(rec)
	if err != nil {
		return destination{}, err
	}
	key := path.Join(prefix, name)
	if r.option.Gzip && !strings.HasSuffix(name, gzipSuffix) {
		key = key + gzipSuffix
	}
	return destination{
		Bucket: r.option.Bucket,
		Key:    key,
	}, nil
}

func (r *Router) putToS3(ctx context.Context, dest destination, body io.ReadSeeker, meta map[string]string) error {
	r.sem.Acquire(ctx, 1)
	defer r.sem.Release(1)

	s3c, err := r.s3Client(ctx, dest.Bucket)
	if err != nil {
		return err
	}

	in := &s3.PutObjectInput{
		Bucket:   &dest.Bucket,
		Key:      &dest.Key,
		Body:     body,
		Metadata: meta,
	}
	log.Println("[info] starting put to", dest.String())
	if _, err := s3c.PutObject(ctx, in); err == nil {
		log.Println("[info] completed put to", dest.String())
	}
	return err
}

func (r *Router) defaultS3Client() *s3.Client {
	return r.s3[r.awsConf.Region]
}

// s3Client returns s3 client for same region as the bucket
func (r *Router) s3Client(ctx context.Context, bucket string) (*s3.Client, error) {
	bucketRegion, err := r.getS3BucketRegion(ctx, bucket)
	if err != nil {
		return nil, err
	}

	r.s3Lock.Lock()
	r.s3Lock.Unlock()

	if s3, ok := r.s3[bucketRegion]; ok {
		return s3, nil
	}

	awsConfig := r.awsConf.Copy()
	awsConfig.Region = bucketRegion
	s3 := s3.NewFromConfig(awsConfig)
	r.s3[bucketRegion] = s3

	return s3, nil
}

func (r *Router) getS3BucketRegion(ctx context.Context, bucket string) (string, error) {
	r.s3bucketRegionLock.Lock()
	defer r.s3bucketRegionLock.Unlock()

	if region, ok := r.s3bucketRegion[bucket]; ok {
		log.Printf("[debug] bucket region for %s is cached: %s\n", bucket, region)
		return region, nil
	}

	region, err := manager.GetBucketRegion(ctx, r.defaultS3Client(), bucket)
	if err != nil {
		return "", err
	}

	r.s3bucketRegion[bucket] = region
	log.Printf("[debug] bucket region for %s is %s, added to cache\n", bucket, region)

	return region, nil
}

type record struct {
	parsed map[string]interface{}
	raw    []byte
}

func newRecord(raw []byte) *record {
	return &record{
		parsed: make(map[string]interface{}),
		raw:    raw,
	}
}

type destination struct {
	Bucket string
	Key    string
}

func (d destination) String() string {
	u := url.URL{
		Scheme: "s3",
		Host:   d.Bucket,
		Path:   d.Key,
	}
	return u.String()
}
