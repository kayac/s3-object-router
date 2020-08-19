package router

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
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

// LF represents LineFeed \n
var LF = []byte("\n")

// MaxConcurrency represents maximum concurrency for uploading to S3
var MaxConcurrency = 10

// Router represents s3-object-router application
type Router struct {
	s3     *s3.S3
	option *Option
	sem    *semaphore.Weighted

	genKeyPrefix func(record) (string, error)
}

// New creates a new router
func New(opt *Option, sess *session.Session) (*Router, error) {
	if err := opt.Validate(); err != nil {
		return nil, err
	}

	t, err := template.New("prefixGenerator").Parse(opt.KeyPrefix)
	if err != nil {
		return nil, err
	}

	return &Router{
		s3:     s3.New(sess),
		option: opt,
		sem:    semaphore.NewWeighted(int64(MaxConcurrency)),
		genKeyPrefix: func(r record) (string, error) {
			var b strings.Builder
			if err := t.Execute(&b, r); err != nil {
				return "", err
			}
			return b.String(), nil
		},
	}, nil
}

// Run runs router
func (r *Router) Run(ctx context.Context, s3url string) error {
	src, key, err := r.getS3Object(s3url)
	if err != nil {
		return err
	}
	defer src.Close()
	scanner := bufio.NewScanner(src)
	dests := make(map[destination]buffer)

	for scanner.Scan() {
		recordBytes := scanner.Bytes()
		var rec record
		if err := json.Unmarshal(recordBytes, &rec); err != nil {
			log.Println("[warn] failed to parse record", err)
			continue
		}
		d, err := r.genDestination(rec, key)
		if err != nil {
			log.Println("[warn] failed to generate destination", err)
			continue
		}
		body := dests[d]
		if body == nil {
			if r.option.Gzip {
				body = newGzipBuffer()
			} else {
				body = new(bytes.Buffer)
			}
		}
		body.Write(recordBytes)
		body.Write(LF)
		dests[d] = body
	}

	eg := errgroup.Group{}
	for dest, buf := range dests {
		dest, buf := dest, buf
		if c, isCloser := buf.(io.Closer); isCloser {
			c.Close()
		}
		body := bytes.NewReader(buf.Bytes())
		log.Println("[debug]", dest.String(), body.Len())
		eg.Go(func() error {
			return r.uploadToS3(ctx, dest, body)
		})
	}

	return eg.Wait()
}

func (r *Router) getS3Object(s3url string) (io.ReadCloser, string, error) {
	u, err := url.Parse(s3url)
	if err != nil {
		return nil, "", err
	}
	if u.Scheme != "s3" {
		return nil, "", errors.New("s3:// required")
	}

	out, err := r.s3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(u.Host),
		Key:    aws.String(strings.TrimPrefix(u.Path, "/")),
	})
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256([]byte(s3url))
	return out.Body, fmt.Sprintf("%x", sum), nil
}

func (r *Router) genDestination(rec record, base string) (destination, error) {
	prefix, err := r.genKeyPrefix(rec)
	if err != nil {
		return destination{}, err
	}
	key := path.Join(prefix, base)
	if r.option.Gzip {
		key = key + ".gz"
	}
	return destination{
		Bucket: r.option.Bucket,
		Key:    key,
	}, nil
}

func (r *Router) uploadToS3(ctx context.Context, dest destination, body io.ReadSeeker) error {
	r.sem.Acquire(ctx, 1)
	defer r.sem.Release(1)

	in := &s3.PutObjectInput{
		Bucket: &dest.Bucket,
		Key:    &dest.Key,
		Body:   body,
	}
	log.Println("[info] starting upload to", dest.String())
	_, err := r.s3.PutObjectWithContext(ctx, in)
	if err == nil {
		log.Println("[info] completed upload to", dest.String())
	}
	return err
}

type record map[string]interface{}

type buffer interface {
	Write([]byte) (int, error)
	Bytes() []byte
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
