package router

import "io"

var (
// NewXXX = newXXX
)

func DoTestRoute(r *Router, src io.Reader, s3url string) (map[string]string, error) {
	dests, err := r.route(src, s3url)
	if err != nil {
		return nil, err
	}
	res := make(map[string]string, len(dests))
	for dest, buffer := range dests {
		res[dest.String()] = string(buffer.Bytes())
	}
	return res, nil
}
