package router

import "io"

var (
// NewXXX = newXXX
)

func DoTestRoute(r *Router, src io.Reader, key string) (map[string]string, error) {
	dests, err := r.route(src, key)
	if err != nil {
		return nil, err
	}
	res := make(map[string]string, len(dests))
	for dest, buffer := range dests {
		res[dest.String()] = string(buffer.Bytes())
	}
	return res, nil
}
