// Package transport sends the client's requests and retries them after a 429 rate_limited, a 5xx or a network
// error.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// Transport sends GET requests with the key, the User-Agent and the timeout, and retries them.
type Transport struct {
	HTTP       *http.Client
	APIKey     string
	UserAgent  string
	Timeout    time.Duration
	MaxRetries int
}

// Error is a failed request. Status is 0 when no answer came back; Err is then the error under it.
type Error struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration
	Err        error
}

func (e *Error) Error() string {
	return e.Message
}

// Get sends a GET to target until it succeeds, a retry is not allowed, or ctx is done.
func (t *Transport) Get(ctx context.Context, target string, auth bool) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		body, err := t.send(ctx, target, auth)

		if err == nil {
			return body, nil
		}

		delay, retry := t.delay(err, attempt)

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

func (t *Transport) send(ctx context.Context, target string, auth bool) ([]byte, error) {
	if t.Timeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, t.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)

	if err != nil {
		return nil, &Error{Message: err.Error(), Err: err}
	}

	req.Header.Set("User-Agent", t.UserAgent)

	if auth {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
		req.Header.Set("Accept", "application/json")
	}

	resp, err := t.HTTP.Do(req)

	if err != nil {
		return nil, &Error{Message: "could not reach the API: " + err.Error(), Err: err}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, &Error{Message: "could not read the answer: " + err.Error(), Err: err}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}

	return nil, apiError(resp, body)
}

// delay says whether to retry after err, and how long to wait first.
func (t *Transport) delay(err error, attempt int) (time.Duration, bool) {
	var e *Error

	if attempt >= t.MaxRetries || !errors.As(err, &e) {
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
