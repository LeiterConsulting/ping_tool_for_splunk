package config

import (
	"os"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

type endpointFileSignature struct {
	modTime time.Time
	size    int64
}

func (s endpointFileSignature) equal(other endpointFileSignature) bool {
	return s.size == other.size && s.modTime.Equal(other.modTime)
}

type EndpointReloader struct {
	path       string
	current    []models.Endpoint
	lastGood   endpointFileSignature
	lastFailed endpointFileSignature
	hasFailure bool
}

func NewEndpointReloader(path string) (*EndpointReloader, []models.Endpoint, error) {
	endpoints, err := LoadEndpoints(path)
	if err != nil {
		return nil, nil, err
	}
	sig, err := endpointSignature(path)
	if err != nil {
		return nil, nil, err
	}
	reloader := &EndpointReloader{
		path:     path,
		current:  cloneEndpoints(endpoints),
		lastGood: sig,
	}
	return reloader, cloneEndpoints(endpoints), nil
}

func (r *EndpointReloader) ReloadIfChanged() ([]models.Endpoint, bool, error) {
	sig, err := endpointSignature(r.path)
	if err != nil {
		return cloneEndpoints(r.current), false, err
	}
	if sig.equal(r.lastGood) {
		return cloneEndpoints(r.current), false, nil
	}
	if r.hasFailure && sig.equal(r.lastFailed) {
		return cloneEndpoints(r.current), false, nil
	}

	endpoints, err := LoadEndpoints(r.path)
	if err != nil {
		r.lastFailed = sig
		r.hasFailure = true
		return cloneEndpoints(r.current), false, err
	}

	r.current = cloneEndpoints(endpoints)
	r.lastGood = sig
	r.hasFailure = false
	return cloneEndpoints(r.current), true, nil
}

func endpointSignature(path string) (endpointFileSignature, error) {
	st, err := os.Stat(path)
	if err != nil {
		return endpointFileSignature{}, err
	}
	return endpointFileSignature{modTime: st.ModTime().UTC(), size: st.Size()}, nil
}

func cloneEndpoints(in []models.Endpoint) []models.Endpoint {
	if len(in) == 0 {
		return nil
	}
	out := make([]models.Endpoint, len(in))
	copy(out, in)
	return out
}