package pve

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/fjacquet/pve_exporter/internal/models"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

const (
	defaultTimeout   = 30 * time.Second
	retryCount       = 2
	retryWaitTime    = 500 * time.Millisecond
	retryMaxWaitTime = 3 * time.Second
)

// Doer performs unwrapped GET requests against one PVE target.
type Doer interface {
	// Get fetches path and unmarshals the response "data" field into out.
	Get(ctx context.Context, path string, out interface{}) error
	// GetOptional is Get for best-effort endpoints; a 403/404 response is not
	// counted toward RequestErrors (see GetOptional on *Client).
	GetOptional(ctx context.Context, path string, out interface{}) error
	// Name returns the configured target (cluster) name.
	Name() string
	// RequestErrors returns the cumulative count of failed PVE API requests.
	RequestErrors() int64
}

// Client is a lean resty-based PVE API client using static API-token auth.
type Client struct {
	name          string
	http          *resty.Client
	requestErrors atomic.Int64
}

// envelope models the {"data": ...} wrapper every PVE endpoint returns.
type envelope struct {
	Data json.RawMessage `json:"data"`
}

// NewClient builds a Client for one target. When trace is true, response bodies
// are logged; this is safe because the API token lives only in the request
// header (never echoed in a PVE response body).
func NewClient(cfg models.ClusterConfig, trace bool) *Client {
	httpClient := resty.New().
		SetBaseURL(cfg.BaseURL()).
		SetHeader("Authorization", cfg.AuthHeader()).
		SetHeader("Accept", "application/json").
		SetTimeout(defaultTimeout).
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWaitTime).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true // transport error: retry
			}
			// Retry rate-limiting and transient server errors; never 4xx.
			return r.StatusCode() == http.StatusTooManyRequests || r.StatusCode() >= 500
		})

	httpClient.SetTLSClientConfig(&tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // operator opt-in for self-signed PVE certs
		MinVersion:         tls.VersionTLS12,
	})

	if trace {
		httpClient.OnAfterResponse(func(_ *resty.Client, r *resty.Response) error {
			log.WithFields(log.Fields{
				"cluster": cfg.Name,
				"method":  r.Request.Method,
				"path":    r.Request.URL,
				"status":  r.StatusCode(),
			}).Infof("API trace:\n%s", r.Body())
			return nil
		})
	}

	return &Client{name: cfg.Name, http: httpClient}
}

// Name returns the target name.
func (c *Client) Name() string { return c.name }

// RequestErrors returns the cumulative count of failed PVE API requests.
func (c *Client) RequestErrors() int64 { return c.requestErrors.Load() }

// Get fetches path and unmarshals the "data" field into out. Any transport
// error or non-200 response counts toward RequestErrors.
func (c *Client) Get(ctx context.Context, path string, out interface{}) error {
	return c.get(ctx, path, out, false)
}

// GetOptional is Get for best-effort endpoints whose absence is not an error.
// A 403 (permission denied) or 404 (feature absent) is expected on restricted
// tokens or feature-less clusters and is NOT counted toward RequestErrors; all
// other failures (transport, 5xx, other 4xx) still count. The error is still
// returned, so callers keep their existing "log debug and continue" handling.
func (c *Client) GetOptional(ctx context.Context, path string, out interface{}) error {
	return c.get(ctx, path, out, true)
}

// get is the shared implementation for Get and GetOptional. When optional is
// true, an expected-absence status (403/404) does not increment RequestErrors.
func (c *Client) get(ctx context.Context, path string, out interface{}, optional bool) error {
	resp, err := c.http.R().SetContext(ctx).Get(path)
	if err != nil {
		// resty has already exhausted its retry budget; this counts one logical
		// call failure, not the number of individual wire attempts. A transport
		// failure always counts, even for optional endpoints.
		c.requestErrors.Add(1)
		return fmt.Errorf("GET %s: %w", path, err)
	}
	if code := resp.StatusCode(); code != http.StatusOK {
		// Counted once per logical call after all retries, unless this is an
		// optional endpoint returning an expected-absence status (403/404).
		if !(optional && isExpectedAbsence(code)) {
			c.requestErrors.Add(1)
		}
		return fmt.Errorf("GET %s: unexpected status %d", path, code)
	}
	var env envelope
	if err := json.Unmarshal(resp.Body(), &env); err != nil {
		return fmt.Errorf("GET %s: decode envelope: %w", path, err)
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("GET %s: decode data: %w", path, err)
	}
	return nil
}

// isExpectedAbsence reports whether an HTTP status represents an optional
// endpoint that is unavailable by permission (403) or absence (404), as opposed
// to a genuine API failure that should count toward RequestErrors.
func isExpectedAbsence(code int) bool {
	return code == http.StatusForbidden || code == http.StatusNotFound
}
