package router

import "errors"

// Option represents option values of router
type Option struct {
	Bucket     string
	KeyPrefix  string
	TimeParse  bool
	TimeKey    string
	TimeFormat string
	Gzip       bool
}

// Validate validates option values
func (opt *Option) Validate() error {
	if opt.Bucket == "" {
		return errors.New("bucket must not be empty")
	}
	if opt.KeyPrefix == "" {
		return errors.New("key-prefix must not be empty")
	}
	return nil
}
