// Package formflow is a small client for the FormFlow REST API. Standard library only.
package formflow

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	mrand "math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultVersion = "2026-10-01"

// Options configures a Client. Zero values pick the defaults noted per field.
type Options struct {
	APIKey      string
	BaseURL     string        // default https://app.formflow.dev
	Version     string        // default DefaultVersion
	MaxRetries  int           // retries after the first attempt; default 3. Use -1 for none.
	Timeout     time.Duration // per attempt; default 30s
	WorkspaceID string        // sent as FormFlow-Workspace
	HTTPClient  *http.Client
	Sleep       func(context.Context, time.Duration) error // for tests
}

type Client struct {
	o Options
}

// Page is one page of a list endpoint.
type Page[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"nextCursor"`
}

// Request describes one API call. IdempotencyKey is generated for POST when empty and reused across retries.
type Request struct {
	Method, Path   string
	Query          url.Values
	Body           any
	IdempotencyKey string
}

func New(o Options) (*Client, error) {
	if o.APIKey == "" {
		return nil, errors.New("formflow: APIKey is required")
	}
	if o.BaseURL == "" {
		o.BaseURL = "https://app.formflow.dev"
	}
	o.BaseURL = strings.TrimRight(o.BaseURL, "/")
	if o.Version == "" {
		o.Version = DefaultVersion
	}
	switch {
	case o.MaxRetries == 0:
		o.MaxRetries = 3
	case o.MaxRetries < 0:
		o.MaxRetries = 0
	}
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{}
	}
	if o.Sleep == nil {
		o.Sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	return &Client{o: o}, nil
}

func backoff(attempt int) time.Duration {
	d := math.Min(30, 0.5*math.Pow(2, float64(attempt)))
	return time.Duration(d * (0.75 + mrand.Float64()*0.5) * float64(time.Second))
}

func newKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "ff-" + hex.EncodeToString(b)
}

// Do sends the request, retrying 429, 5xx and network errors with exponential backoff (Retry-After wins),
// and decodes the JSON envelope into out (pass nil to ignore). A 204 leaves out untouched.
func (c *Client) Do(ctx context.Context, r Request, out any) error {
	u := c.o.BaseURL + "/api/v1" + r.Path
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}
	var payload []byte
	if r.Body != nil {
		var err error
		if payload, err = json.Marshal(r.Body); err != nil {
			return err
		}
	}
	if r.Method == http.MethodPost && r.IdempotencyKey == "" {
		r.IdempotencyKey = newKey()
	}
	for attempt := 0; ; attempt++ {
		actx, cancel := context.WithTimeout(ctx, c.o.Timeout)
		req, err := http.NewRequestWithContext(actx, r.Method, u, bytes.NewReader(payload))
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.o.APIKey)
		req.Header.Set("FormFlow-Version", c.o.Version)
		req.Header.Set("Accept", "application/json")
		if c.o.WorkspaceID != "" {
			req.Header.Set("FormFlow-Workspace", c.o.WorkspaceID)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if r.IdempotencyKey != "" {
			req.Header.Set("Idempotency-Key", r.IdempotencyKey)
		}
		res, err := c.o.HTTPClient.Do(req)
		if err != nil {
			cancel()
			if ctx.Err() != nil { // the caller cancelled: never retry
				return ctx.Err()
			}
			if attempt >= c.o.MaxRetries {
				return &Error{Code: "network_error", Message: err.Error()}
			}
			if serr := c.o.Sleep(ctx, backoff(attempt)); serr != nil {
				return serr
			}
			continue
		}
		raw, rerr := io.ReadAll(res.Body)
		res.Body.Close()
		cancel()
		if rerr != nil {
			return rerr
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			if res.StatusCode == http.StatusNoContent || out == nil || len(raw) == 0 {
				return nil
			}
			return json.Unmarshal(raw, out)
		}
		retryAfter, _ := strconv.ParseFloat(res.Header.Get("Retry-After"), 64)
		if (res.StatusCode == 429 || res.StatusCode >= 500) && attempt < c.o.MaxRetries {
			wait := backoff(attempt)
			if retryAfter > 0 {
				wait = time.Duration(retryAfter * float64(time.Second))
			}
			if serr := c.o.Sleep(ctx, wait); serr != nil {
				return serr
			}
			continue
		}
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Details any    `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &env)
		e := &Error{Status: res.StatusCode, Code: env.Error.Code, Message: env.Error.Message, Details: env.Error.Details, RequestID: res.Header.Get("X-Request-Id"), RetryAfter: retryAfter}
		if e.Code == "" {
			e.Code, e.Message = "http_error", http.StatusText(res.StatusCode)
		}
		return e
	}
}

// Data runs a request and decodes the `data` member of the envelope into T.
func Data[T any](ctx context.Context, c *Client, r Request) (T, error) {
	var env struct {
		Data T `json:"data"`
	}
	err := c.Do(ctx, r, &env)
	return env.Data, err
}

// List fetches one page.
func List[T any](ctx context.Context, c *Client, path string, q url.Values) (Page[T], error) {
	var p Page[T]
	err := c.Do(ctx, Request{Method: http.MethodGet, Path: path, Query: q}, &p)
	return p, err
}

// Each walks every page and calls fn per item. Return an error from fn to stop early.
func Each[T any](ctx context.Context, c *Client, path string, q url.Values, fn func(T) error) error {
	q2 := url.Values{}
	for k, v := range q {
		q2[k] = v
	}
	for {
		p, err := List[T](ctx, c, path, q2)
		if err != nil {
			return err
		}
		for _, it := range p.Data {
			if err := fn(it); err != nil {
				return err
			}
		}
		if p.NextCursor == nil || *p.NextCursor == "" {
			return nil
		}
		q2.Set("cursor", *p.NextCursor)
	}
}
