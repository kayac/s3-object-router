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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
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
	s3     *s3.S3
	option *Option
	sem    *semaphore.Weighted

	genKeyPrefix func(record) (string, error)
}

// New creates a new router
func New(opt *Option) (*Router, error) {
	if err := opt.Init(); err != nil {
		return nil, err
	}

	sess, err := session.NewSession()
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
		s3:     s3.New(sess),
		option: opt,
		sem:    semaphore.NewWeighted(int64(MaxConcurrency)),
		genKeyPrefix: func(r record) (string, error) {
			var b strings.Builder
			if err := tmpl.Execute(&b, r); err != nil {
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
	dests, err := r.route(src, s3url)
	if err != nil {
		return err
	}

	meta := map[string]*string{
		MetaHeaderName: aws.String(s3url),
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

func (r *Router) genKey(s3url string) string {
	if r.option.KeepOriginalName {
		return path.Base(s3url)
	}
	sum := sha256.Sum256([]byte(s3url))
	return fmt.Sprintf("%x", sum)
}

func (r *Router) route(src io.Reader, s3url string) (map[destination]buffer, error) {
	key := r.genKey(s3url)
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
		var rec record
		if err := recordParser.Parse(recordBytes, &rec); err != nil {
			if err != SkipLine {
				log.Println("[warn] failed to parse record", err)
			}
			continue
		}
		if r.option.TimeParse {
			if ts, ok := rec[r.option.TimeKey].(string); ok {
				rec[r.option.TimeKey], err = r.option.timeParser.Parse(ts)
				if err != nil {
					log.Println("[warn] failed to parse time", err)
				}
			}
		}
		d, err := r.genDestination(rec, key)
		if err != nil {
			log.Println("[warn] failed to generate destination", err)
			continue
		}
		enc := encs[d]
		if enc == nil {
			var body buffer
			if r.option.Gzip {
				body = newGzipBuffer()
			} else {
				body = new(bytes.Buffer)
			}
			enc = r.option.newEncoder(body)
		}
		if err := enc.Encode(rec, recordBytes); err != nil {
			log.Println("[warn] failed to encode %s record: %v", err, rec)
			continue
		}
		encs[d] = enc
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

	out, err := r.s3.GetObjectWithContext(ctx, &s3.GetObjectInput{
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
			return nil, fmt.Errorf("%s seems to be an already routed object. original: %s", s3url, *value)
		}
	}

	return out.Body, nil
}

func (r *Router) genDestination(rec record, name string) (destination, error) {
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

func (r *Router) putToS3(ctx context.Context, dest destination, body io.ReadSeeker, meta map[string]*string) error {
	r.sem.Acquire(ctx, 1)
	defer r.sem.Release(1)

	in := &s3.PutObjectInput{
		Bucket:   &dest.Bucket,
		Key:      &dest.Key,
		Body:     body,
		Metadata: meta,
	}
	log.Println("[info] starting put to", dest.String())
	_, err := r.s3.PutObjectWithContext(ctx, in)
	if err == nil {
		log.Println("[info] completed put to", dest.String())
	}
	return err
}

type record map[string]interface{}

type buffer interface {
	Write([]byte) (int, error)
	Bytes() []byte
}

type encoder interface {
	Encode(record, []byte) error
	Buffer() buffer
}

type gzBuffer struct {
	bytes.Buffer
	gz *gzip.Writer
}

func newGzipBuffer() *gzBuffer {
	buf := &gzBuffer{}
	buf.gz = gzip.NewWriter(&buf.Buffer)
	return buf
}

func (buf *gzBuffer) Write(p []byte) (int, error) {
	return buf.gz.Write(p)
}

func (buf *gzBuffer) Close() error {
	return buf.gz.Close()
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
