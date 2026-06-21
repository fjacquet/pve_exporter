package pve

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
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
	// Name returns the configured target (cluster) name.
	Name() string
}

// Client is a lean resty-based PVE API client using static API-token auth.
type Client struct {
	name string
	http *resty.Client
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

// Get fetches path and unmarshals the "data" field into out.
func (c *Client) Get(ctx context.Context, path string, out interface{}) error {
	resp, err := c.http.R().SetContext(ctx).Get(path)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode())
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
