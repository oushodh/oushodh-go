package oushodh

import (
	"errors"
	"fmt"
	"time"
)

// Error is every error the client returns after it sends a request.
//
// Status is the HTTP status, or 0 when the request never got an answer (a network fault or a timeout).
// Code is the API's error code, such as "not_found" or "plan_limit". Message is for people: match on
// Code or on the sentinel errors with errors.Is, never on Message.
type Error struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration

	err error
}

// Sentinel errors to use with errors.Is. A family matches every error with that status; a code matches
// only that code.
var (
	ErrBadRequest    = errors.New("oushodh: bad request")       // 400
	ErrUnauthorized  = errors.New("oushodh: unauthorized")      // 401
	ErrForbidden     = errors.New("oushodh: forbidden")         // 403: plan_expired or plan_limit
	ErrPlanExpired   = errors.New("oushodh: plan expired")      // code plan_expired
	ErrPlanLimit     = errors.New("oushodh: plan limit")        // code plan_limit
	ErrNotFound      = errors.New("oushodh: not found")         // 404
	ErrRateLimited   = errors.New("oushodh: too many requests") // 429: rate_limited or quota_exceeded
	ErrQuotaExceeded = errors.New("oushodh: quota exceeded")    // code quota_exceeded
	ErrServer        = errors.New("oushodh: server error")      // 5xx
	ErrNetwork       = errors.New("oushodh: network error")     // no answer, including timeouts
)

func (e *Error) Error() string {
	if e.Code == "" {
		return "oushodh: " + e.Message
	}

	return fmt.Sprintf("oushodh: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// Unwrap gives the network error under a Status 0 error, so errors.Is(err, context.DeadlineExceeded) works.
func (e *Error) Unwrap() error {
	return e.err
}

func (e *Error) Is(target error) bool {
	switch target {
	case ErrBadRequest:
		return e.Status == 400
	case ErrUnauthorized:
		return e.Status == 401
	case ErrForbidden:
		return e.Status == 403
	case ErrPlanExpired:
		return e.Code == "plan_expired"
	case ErrPlanLimit:
		return e.Code == "plan_limit"
	case ErrNotFound:
		return e.Status == 404
	case ErrRateLimited:
		return e.Status == 429
	case ErrQuotaExceeded:
		return e.Code == "quota_exceeded"
	case ErrServer:
		return e.Status >= 500
	case ErrNetwork:
		return e.Status == 0
	}

	return false
}
