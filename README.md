# Oushodh Go SDK

The official Go client for the Oushodh API.

[![CI](https://github.com/oushodh/oushodh-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/oushodh/oushodh-go/actions/workflows/ci.yml)
[![Licence: MIT](https://img.shields.io/github/license/oushodh/oushodh-go)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/oushodh/oushodh-go)](go.mod)

## What Oushodh is

Oushodh serves Bangladesh medicine data over a REST API: brands, generics, prices and makers. Make a key in the member
portal at [app.oushodh.dev](https://app.oushodh.dev), and read the docs at [oushodh.dev/docs](https://oushodh.dev/docs).

The SDK needs Go 1.27 or newer. It uses only the standard library, and every call takes a `context.Context`.

## Install

Install it with `go get`:

```sh
go get github.com/oushodh/oushodh-go
```

## Quickstart

Make a key on the Keys page of the member portal. A key starts with `oush_` and is shown once. Keep it on your
server: never put it in a web page or an app.

```sh
export OUSHODH_API_KEY=oush_...
```

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/oushodh/oushodh-go"
)

func main() {
	ctx := context.Background()

	client, err := oushodh.NewClient() // reads OUSHODH_API_KEY

	if err != nil {
		log.Fatal(err)
	}

	result, err := client.Search(ctx, "paracetamol", 5)

	if err != nil {
		log.Fatal(err)
	}

	for _, medicine := range result.Items {
		fmt.Println(medicine.ID, medicine.ProductName)
	}

	medicine, err := client.GetMedicine(ctx, "MED-012933")

	if err != nil {
		log.Fatal(err)
	}

	if unit := medicine.Prices.Unit; unit != nil {
		fmt.Println(medicine.ProductName, unit.Amount, "BDT a unit")
	}
}
```

`oushodh.NewClient(oushodh.WithAPIKey("oush_..."))` takes the key directly. With no key and no `OUSHODH_API_KEY`,
`NewClient` returns an error.

| Method                         | Route                          | Returns                      |
| ------------------------------ | ------------------------------ | ---------------------------- |
| `Meta(ctx)`                    | `GET /v1/meta`                 | `*Meta`                      |
| `ListMedicines(ctx, params)`   | `GET /v1/medicines`            | `*Page`                      |
| `Medicines(ctx, params)`       | `GET /v1/medicines`, each page | `iter.Seq2[Medicine, error]` |
| `Search(ctx, q, limit)`        | `GET /v1/medicines/search`     | `*SearchResult`              |
| `GetMedicine(ctx, ref)`        | `GET /v1/medicines/{ref}`      | `*MedicineDetail`            |
| `ImageURL(file)`               | none                           | the picture's address        |
| `GetImage(ctx, file)`          | `GET /v1/images/{file}`        | the picture's bytes          |

`Meta` names your account and counts the medicines your plan reads. Call it to check that a key works.

`GetMedicine` takes an ID in any profile, an old ID or a UUID. The answer always carries the medicine's current ID,
and its `Monograph`.

`ListParams` has `Limit`, `Cursor`, `UpdatedSince` and `Q`. A zero value leaves the option out. `Search` takes a
`limit` of 1 to 50, or `0` for the default of 20.

A value no source gives is `null` in the API and `nil` in Go: such fields are pointers. Each model has an `Extra` map
that holds fields the API adds later, by name, as raw JSON. A client is safe to use from many goroutines.

## Paging

`ListMedicines` returns one page: `Items`, `Next` and `Total`. Pass `*Next` back as `Cursor` for the next page. `Next`
is `nil` on the last page. `Limit` is 1 to 500, and 100 when you leave it out.

```go
params := oushodh.ListParams{Limit: 500}

for {
	page, err := client.ListMedicines(ctx, params)

	if err != nil {
		return err
	}

	for _, medicine := range page.Items {
		fmt.Println(medicine.ID, medicine.ProductName)
	}

	if page.Next == nil {
		break
	}

	params.Cursor = *page.Next
}
```

`Medicines` does the same for you. It yields every medicine and follows `Next` to the end. It stops at the first
error and yields it:

```go
for medicine, err := range client.Medicines(ctx, oushodh.ListParams{Limit: 500}) {
	if err != nil {
		return err
	}

	fmt.Println(medicine.ID, medicine.ProductName)
}
```

Search has no cursor. To page through every match, use `Medicines` with `Q`.

## Keeping a copy in sync

Copy every medicine once, then fetch only what changed.

1. The first sync pages through every medicine and keeps the newest `LastUpdated`.
2. Each later sync passes that time as `UpdatedSince` and pages to the end. It gets the medicines changed at or after
   that time, oldest change first.

```go
since := loadLastSync() // the zero time on the first sync

for medicine, err := range client.Medicines(ctx, oushodh.ListParams{Limit: 500, UpdatedSince: since}) {
	if err != nil {
		return err
	}

	saveMedicine(medicine.UUID, medicine)

	if medicine.LastUpdated.After(since) {
		since = medicine.LastUpdated
	}
}

saveLastSync(since)
```

- `UpdatedSince` is a `time.Time`. The client sends it in UTC, to the whole second, such as `2026-11-08T09:30:00Z`.
  The zero time leaves it out.
- "At or after" means a change in the same second is never missed. The medicine that set the time comes back once
  more, so save by `UUID`.
- Store `UUID` as the key: it never changes. Show `ID` to people.
- Medicines are never deleted and IDs are never reused, so a sync never has to remove one.

## Images

`ProductImage` is the address of the medicine's picture, or `nil`. `Images` lists every picture, `ProductImage`
first. An address under `/v1/images/` is a copy Oushodh keeps. It needs no key, so an `<img>` tag can show it, and it
never changes. Any other address belongs to the source, and it can move or be removed.

`GetImage` takes the file name, not the whole address, and sends no key:

```go
if url := medicine.ProductImage; url != nil && strings.HasPrefix(*url, client.ImageURL("")) {
	jpeg, err := client.GetImage(ctx, path.Base(*url))

	if err != nil {
		return err
	}

	save(jpeg)
}
```

`ImageURL(file)` builds the address of a file, such as
`50d858e0985ecc7f60418aaf0cc5ab587f42c2570a884095a9e8ccacd0f6545c.jpg`.

## Errors

Every error from a request is an `*oushodh.Error`. It has `Status` (the HTTP status, or `0` when no answer came back),
`Code` (the API's error code), `Message` and `RetryAfter`. Match on `Code` or on the sentinel errors with `errors.Is`,
never on `Message`: the message is for people and may change.

| Sentinel           | Status | `Code`                                             |
| ------------------ | ------ | -------------------------------------------------- |
| `ErrBadRequest`    | 400    | `bad_request`                                      |
| `ErrUnauthorized`  | 401    | `unauthorized`                                     |
| `ErrForbidden`     | 403    | `plan_expired` or `plan_limit`                     |
| `ErrPlanExpired`   | 403    | `plan_expired`                                     |
| `ErrPlanLimit`     | 403    | `plan_limit`                                       |
| `ErrNotFound`      | 404    | `not_found`                                        |
| `ErrRateLimited`   | 429    | `rate_limited` or `quota_exceeded`                 |
| `ErrQuotaExceeded` | 429    | `quota_exceeded`                                   |
| `ErrServer`        | 5xx    | `internal`                                         |
| `ErrNetwork`       | 0      | none: the network failed or the request timed out  |

A network error wraps the error under it, so `errors.Is(err, context.DeadlineExceeded)` is true after a timeout. When
an error body is not JSON, `Code` is empty and `Message` is `HTTP <status>`. The key never appears in an error
message.

```go
medicine, err := client.GetMedicine(ctx, "MED-999999")

switch {
case errors.Is(err, oushodh.ErrNotFound):
	medicine = nil // No medicine has that ID.
case errors.Is(err, oushodh.ErrPlanLimit):
	return fmt.Errorf("the medicine is beyond your plan: %w", err)
case err != nil:
	return err
}
```

## Retries and rate limits

Your plan sets how many requests each key may make in a minute (60 a minute on Developer). It also sets how many
requests your account may make in a month. See [Rate limits](https://oushodh.dev/docs/rate-limits).

- Over the minute's limit, the API answers `429 rate_limited`. The `Retry-After` header says how many seconds to wait.
  The client waits that long and tries again, up to the `WithMaxRetries` count. After that it returns the error with
  the wait in `RetryAfter`.
- When the month's requests are used up, the API answers `429 quota_exceeded` until the next month. The client never
  retries it. It returns the error at once, and `RetryAfter` holds the time to the next month.
- After a `5xx` or a network error, the client backs off and tries again, up to the `WithMaxRetries` count. Each wait
  is about twice the last, with a random part, and at most 8 seconds.
- The client never retries a `400`, `401`, `403` or `404`: the same request gets the same answer.
- It stops waiting when the context is done.

Every route is a `GET`, so a retry is always safe.

## Plan limits

Developer reads the first 5,000 medicines and Team the first 20,000, in the order of their permanent number. Agency
and Custom read all of them.

Lists and search leave out the medicines beyond your plan's number. A lookup by ID of such a medicine answers
`403 plan_limit`: the client returns an error that matches `ErrPlanLimit`. An account with no active plan gets
`403 plan_expired` (`ErrPlanExpired`) on every call.

## Configuration

Pass options to `NewClient`:

```go
client, err := oushodh.NewClient(
	oushodh.WithAPIKey("oush_..."),
	oushodh.WithTimeout(10*time.Second),
	oushodh.WithMaxRetries(3),
)
```

| Option              | Default                   | What it does                                                    |
| ------------------- | ------------------------- | --------------------------------------------------------------- |
| `WithAPIKey(key)`   | `OUSHODH_API_KEY`         | Your API key.                                                   |
| `WithBaseURL(url)`  | `https://api.oushodh.dev` | Where the API is.                                               |
| `WithTimeout(d)`    | 30 seconds                | How long each request may take. `0` means no limit.             |
| `WithMaxRetries(n)` | `2`                       | Retries after a `429 rate_limited`, a `5xx` or a network error. |
| `WithHTTPClient(c)` | `http.DefaultClient`      | Your own `*http.Client`, for example with a test transport.     |

In your own tests, point `WithBaseURL` at an `httptest.Server`, so no request leaves your machine.

## Development

```sh
go vet ./...
gofmt -l .
go test -race ./...
```

The tests use `net/http/httptest`. They never call the API.

## Licence

MIT. Copyright Oushodh. See [LICENSE](LICENSE).
