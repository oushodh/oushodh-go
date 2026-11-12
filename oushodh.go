// Package oushodh is the Go client for the Oushodh API: medicine data for Bangladesh, checked against the
// DGDA register.
//
// The API reference is at https://oushodh.dev/docs.
package oushodh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK's version. It goes in the User-Agent header.
const Version = "0.1.0"

const (
	DefaultBaseURL    = "https://api.oushodh.dev"
	DefaultTimeout    = 30 * time.Second
	DefaultMaxRetries = 2
)

// Client calls the Oushodh API. It is safe to use from many goroutines.
type Client struct {
	apiKey     string
	baseURL    string
	timeout    time.Duration
	maxRetries int
	http       *http.Client
}

// Option sets one of the client's options in NewClient.
type Option func(*Client)

// WithAPIKey sets the API key. Without it, NewClient reads OUSHODH_API_KEY.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithBaseURL sets where the API is. The default is https://api.oushodh.dev.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithHTTPClient sets the http.Client that sends every request. The default is http.DefaultClient.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) { c.http = client }
}

// WithTimeout sets how long each request may take, retries apart. The default is 30 seconds; 0 means no limit.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) { c.timeout = timeout }
}

// WithMaxRetries sets how many times a request is retried after a 429, a 5xx or a network error. The default is 2.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// NewClient makes a client. It returns an error when there is no API key: none passed and no OUSHODH_API_KEY.
func NewClient(opts ...Option) (*Client, error) {
	c := &Client{
		baseURL:    DefaultBaseURL,
		timeout:    DefaultTimeout,
		maxRetries: DefaultMaxRetries,
		http:       http.DefaultClient,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.apiKey == "" {
		c.apiKey = os.Getenv("OUSHODH_API_KEY")
	}

	if c.apiKey == "" {
		return nil, errors.New("oushodh: no API key: pass WithAPIKey or set the OUSHODH_API_KEY environment variable")
	}

	return c, nil
}

// ListParams are the options of ListMedicines and Medicines. A zero value leaves the option out.
type ListParams struct {
	// Limit is the medicines on one page, 1 to 500. The API's default is 100.
	Limit int
	// Cursor is the Next value of the page before. Medicines starts from it.
	Cursor string
	// UpdatedSince keeps only medicines changed at or after this time, oldest change first. It is sent in
	// UTC to the second, such as 2026-11-08T09:30:00Z.
	UpdatedSince time.Time
	// Q keeps only medicines whose brand, generic or manufacturer starts with these words, or whose ID matches.
	Q string
}

// Meta returns the account the key belongs to. Use it to check that a key works.
func (c *Client) Meta(ctx context.Context) (*Meta, error) {
	var meta Meta

	if err := c.getJSON(ctx, "/v1/meta", nil, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// ListMedicines returns one page of medicines, in the order of their permanent number.
func (c *Client) ListMedicines(ctx context.Context, params ListParams) (*Page, error) {
	query := url.Values{}

	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	}

	if params.Cursor != "" {
		query.Set("cursor", params.Cursor)
	}

	if !params.UpdatedSince.IsZero() {
		query.Set("updated_since", params.UpdatedSince.UTC().Format("2006-01-02T15:04:05Z"))
	}

	if params.Q != "" {
		query.Set("q", params.Q)
	}

	var page Page

	if err := c.getJSON(ctx, "/v1/medicines", query, &page); err != nil {
		return nil, err
	}

	return &page, nil
}

// Medicines returns every medicine, page after page to the end. It stops at the first error and yields it.
func (c *Client) Medicines(ctx context.Context, params ListParams) iter.Seq2[Medicine, error] {
	return func(yield func(Medicine, error) bool) {
		for {
			page, err := c.ListMedicines(ctx, params)

			if err != nil {
				yield(Medicine{}, err)

				return
			}

			for _, medicine := range page.Items {
				if !yield(medicine, nil) {
					return
				}
			}

			if page.Next == nil {
				return
			}

			params.Cursor = *page.Next
		}
	}
}

// Search returns the best matches for a person typing a name, best match first. limit is 1 to 50; 0 leaves
// it to the API's default of 20.
func (c *Client) Search(ctx context.Context, q string, limit int) (*SearchResult, error) {
	query := url.Values{"q": {q}}

	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	var result SearchResult

	if err := c.getJSON(ctx, "/v1/medicines/search", query, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetMedicine returns one medicine by its ID in any profile, an old ID, or its UUID. The answer always
// carries the medicine's current ID.
func (c *Client) GetMedicine(ctx context.Context, ref string) (*MedicineDetail, error) {
	var detail MedicineDetail

	if err := c.getJSON(ctx, "/v1/medicines/"+url.PathEscape(ref), nil, &detail); err != nil {
		return nil, err
	}

	return &detail, nil
}

// ImageURL returns the address of a picture, as product_image gives it. It needs no key.
func (c *Client) ImageURL(file string) string {
	return c.baseURL + "/v1/images/" + url.PathEscape(file)
}

// GetImage returns a picture's JPEG bytes. It sends no key.
func (c *Client) GetImage(ctx context.Context, file string) ([]byte, error) {
	return c.get(ctx, "/v1/images/"+url.PathEscape(file), nil, false)
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, v any) error {
	body, err := c.get(ctx, path, query, true)

	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("oushodh: the answer is not the JSON expected: %w", err)
	}

	return nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, auth bool) ([]byte, error) {
	target := c.baseURL + path

	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	for attempt := 0; ; attempt++ {
		body, err := c.send(ctx, target, auth)

		if err == nil {
			return body, nil
		}

		delay, retry := c.delay(err, attempt)

		if !retry || ctx.Err() != nil {
			return nil, err
		}

		timer := time.NewTimer(delay)

		select {
		case <-ctx.Done():
			timer.Stop()

			return nil, err
		case <-timer.C:
		}
	}
}

func (c *Client) send(ctx context.Context, target string, auth bool) ([]byte, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)

	if err != nil {
		return nil, &Error{Message: err.Error(), err: err}
	}

	req.Header.Set("User-Agent", "oushodh-go/"+Version)

	if auth {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
	}

	resp, err := c.http.Do(req)

	if err != nil {
		return nil, &Error{Message: "could not reach the API: " + err.Error(), err: err}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, &Error{Message: "could not read the answer: " + err.Error(), err: err}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}

	return nil, apiError(resp, body)
}

// delay says whether to retry after err, and how long to wait first.
func (c *Client) delay(err error, attempt int) (time.Duration, bool) {
	var e *Error

	if attempt >= c.maxRetries || !errors.As(err, &e) {
		return 0, false
	}

	switch {
	// Only rate_limited: quota_exceeded lasts until next month, so it returns at once.
	case e.Status == 429 && e.Code == "rate_limited":
		if e.RetryAfter > 0 {
			return e.RetryAfter, true
		}

		return backoff(attempt), true
	case e.Status >= 500, e.Status == 0:
		return backoff(attempt), true
	}

	return 0, false
}

func backoff(attempt int) time.Duration {
	base := min(500*time.Millisecond<<attempt, 8*time.Second)

	return time.Duration(float64(base) * (0.5 + rand.Float64()/2))
}

func apiError(resp *http.Response, body []byte) *Error {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	e := &Error{Status: resp.StatusCode, Message: "HTTP " + strconv.Itoa(resp.StatusCode)}

	if json.Unmarshal(body, &payload) == nil && payload.Error.Code != "" {
		e.Code = payload.Error.Code
		e.Message = payload.Error.Message
	}

	if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
		e.RetryAfter = time.Duration(seconds) * time.Second
	}

	return e
}
