package transport

import (
	"errors"
	"testing"
	"time"
)

func TestDelay(t *testing.T) {
	tr := &Transport{MaxRetries: 2}

	cases := []struct {
		name  string
		err   error
		retry bool
		wait  time.Duration
	}{
		{"rate_limited waits Retry-After", &Error{Status: 429, Code: "rate_limited", RetryAfter: 7 * time.Second}, true, 7 * time.Second},
		{"quota_exceeded returns at once", &Error{Status: 429, Code: "quota_exceeded", RetryAfter: time.Hour}, false, 0},
		{"429 with no code returns at once", &Error{Status: 429}, false, 0},
		{"5xx backs off", &Error{Status: 503}, true, -1},
		{"network error backs off", &Error{Err: errors.New("down")}, true, -1},
		{"404 returns at once", &Error{Status: 404, Code: "not_found"}, false, 0},
		{"another error type returns at once", errors.New("other"), false, 0},
	}

	for _, c := range cases {
		wait, retry := tr.delay(c.err, 0)

		if retry != c.retry {
			t.Errorf("%s: retry = %v", c.name, retry)
		}

		if c.wait >= 0 && wait != c.wait {
			t.Errorf("%s: wait = %v, want %v", c.name, wait, c.wait)
		}

		if c.wait < 0 && (wait < 250*time.Millisecond || wait > 500*time.Millisecond) {
			t.Errorf("%s: wait = %v, want 250ms to 500ms", c.name, wait)
		}
	}

	if _, retry := tr.delay(&Error{Status: 503}, 2); retry {
		t.Error("retried past MaxRetries")
	}
}
