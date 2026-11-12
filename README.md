# Oushodh for Go

The Go SDK for the Oushodh medicine data API: medicine data for Bangladesh, checked against the DGDA register.

- Go 1.27 or newer.
- No dependencies outside the standard library.
- Every call takes a `context.Context`.

The full API reference is at [oushodh.dev/docs](https://oushodh.dev/docs).

## Install

```sh
go get github.com/oushodh/oushodh-go
```

The repository is private for now. Tell Go not to use the public proxy for it, and make sure `git` can reach it with
your GitHub access:

```sh
go env -w GOPRIVATE=github.com/oushodh/*
```

A public install is coming soon.

## Quickstart

Make an API key on the Keys page of the member portal at [app.oushodh.dev](https://app.oushodh.dev). A key starts
with `oush_` and is shown once. Keep it on your server: a key inside an app can be copied out of it.

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

	client, err := oushodh.NewClient(oushodh.WithAPIKey("oush_...")) // or set OUSHODH_API_KEY

	if err != nil {
		log.Fatal(err)
	}

	meta, err := client.Meta(ctx) // check that the key works

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(meta.Client, meta.Medicines)

	result, err := client.Search(ctx, "napa 500", 5)

	if err != nil {
		log.Fatal(err)
	}

	for _, medicine := range result.Items {
		fmt.Println(medicine.ID, medicine.ProductName)
	}

	detail, err := client.GetMedicine(ctx, "MED-012933")

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(detail.Monograph != nil)
}
```

`NewClient` takes these options:

| Option                 | Default                   | What it does                                           |
| ---------------------- | ------------------------- | ------------------------------------------------------ |
| `WithAPIKey(key)`      | `OUSHODH_API_KEY`         | Your API key. With no key, `NewClient` returns an error. |
| `WithBaseURL(url)`     | `https://api.oushodh.dev` | Where the API is.                                      |
| `WithTimeout(d)`       | 30 seconds                | How long each request may take. `0` means no limit.    |
| `WithMaxRetries(n)`    | `2`                       | Retries after a `429 rate_limited`, a `5xx` or a network error. |
| `WithHTTPClient(c)`    | `http.DefaultClient`      | Your own `*http.Client`, for example with a test transport. |

A client is safe to use from many goroutines.

## Methods

| Method                              | Route                          | Returns                 |
| ----------------------------------- | ------------------------------ | ----------------------- |
| `Meta(ctx)`                         | `GET /v1/meta`                 | `*Meta`                 |
| `ListMedicines(ctx, ListParams{})`  | `GET /v1/medicines`            | `*Page`                 |
| `Medicines(ctx, ListParams{})`      | `GET /v1/medicines`, each page | `iter.Seq2[Medicine, error]` |
| `Search(ctx, q, limit)`             | `GET /v1/medicines/search`     | `*SearchResult`         |
| `GetMedicine(ctx, ref)`             | `GET /v1/medicines/{ref}`      | `*MedicineDetail`       |
| `ImageURL(file)`                    | none                           | the picture's address   |
| `GetImage(ctx, file)`               | `GET /v1/images/{file}`        | the picture's bytes     |

`ListParams` has `Limit`, `Cursor`, `UpdatedSince` and `Q`. A zero value leaves the option out. `Search` takes a
`limit` of 1 to 50, or `0` for the default of 20.

`ref` is a medicine's ID in any profile, an old ID, or its UUID. The answer always carries the medicine's current ID.

A value no source gives is `null` in the API, and `nil` in Go: such fields are pointers. Each model has an `Extra`
map that holds fields the API adds later, by name, as raw JSON.

## Paging

`ListMedicines` gives one page: `Items`, `Next` and `Total`. Pass `*Next` back as `Cursor` for the next page. `Next`
is `nil` on the last page. A page holds 1 to 500 medicines (`Limit`, default 100).

`Medicines` does this for you, page after page to the end. It stops at the first error and yields it:

```go
for medicine, err := range client.Medicines(ctx, oushodh.ListParams{Limit: 500}) {
	if err != nil {
		return err
	}

	save(medicine)
}
```

Search has no cursor. To page every match, use `Medicines` with `Q`.

## Sync

Copy the whole database once, then fetch only what changed.

1. The first sync pages through `Medicines` to the end. Keep the `LastUpdated` of the newest medicine.
2. Each later sync passes that time as `UpdatedSince` and pages to the end. Medicines come oldest change first.

```go
since := loadLastSync() // a time.Time; the zero time the first time

for medicine, err := range client.Medicines(ctx, oushodh.ListParams{Limit: 500, UpdatedSince: since}) {
	if err != nil {
		return err
	}

	upsert(medicine.UUID, medicine)

	if medicine.LastUpdated.After(since) {
		since = medicine.LastUpdated
	}
}

storeLastSync(since)
```

`UpdatedSince` is sent in UTC to the whole second, like `2026-11-08T09:30:00Z`. It means "at or after", so the
medicine that set the time comes back once more. Store each medicine by its `UUID`: it never changes. Medicines are
never deleted and IDs are never reused.

## Errors

Every error from a request is an `*oushodh.Error`. It has `Status` (the HTTP status, or `0` when no answer came),
`Code` (the API's error code), `Message` and `RetryAfter`. Match on `Code` or on the sentinel errors with
`errors.Is`, never on `Message`: the message is for people and may change. When an error body is not JSON, `Code` is
empty and `Message` is `HTTP <status>`.

| Sentinel           | Status | `Code`                             |
| ------------------ | ------ | ---------------------------------- |
| `ErrBadRequest`    | 400    | `bad_request`                      |
| `ErrUnauthorized`  | 401    | `unauthorized`                     |
| `ErrForbidden`     | 403    | `plan_expired` or `plan_limit`     |
| `ErrPlanExpired`   | 403    | `plan_expired`                     |
| `ErrPlanLimit`     | 403    | `plan_limit`                       |
| `ErrNotFound`      | 404    | `not_found`                        |
| `ErrRateLimited`   | 429    | `rate_limited` or `quota_exceeded` |
| `ErrQuotaExceeded` | 429    | `quota_exceeded`                   |
| `ErrServer`        | 5xx    | `internal`                         |
| `ErrNetwork`       | 0      | none: the network failed or the request timed out |

A network error wraps the error under it, so `errors.Is(err, context.DeadlineExceeded)` is true after a timeout.

```go
detail, err := client.GetMedicine(ctx, "MED-012933")

switch {
case errors.Is(err, oushodh.ErrNotFound):
	detail = nil
case errors.Is(err, oushodh.ErrPlanLimit):
	return fmt.Errorf("upgrade your plan to read this medicine: %w", err)
case err != nil:
	return err
}
```

The SDK retries a `429 rate_limited`, a `5xx` and a network error up to the `WithMaxRetries` count. After a `5xx` or a
network error it waits a little longer each time. It never retries any other `4xx`: they give the same answer every
time. It stops waiting when the context is done. The error message never holds your key.

## Rate limits

Your plan sets how many requests each key may make in a minute (60 a minute on Developer), and how many your account
may make in a month.

- Over the minute's limit, the API answers `429 rate_limited`. The `Retry-After` header says how many seconds to wait.
  The SDK waits that long and tries again.
- When the month's requests are used up, the API answers `429 quota_exceeded` until the next month. The SDK returns
  this error at once and never waits. `RetryAfter` holds the time to the start of next month.

## Plan limits

Some plans read only the first medicines, in the order of their permanent number. Lists and search leave out the rest.
A lookup by ID of such a medicine returns `403` with `Code` `plan_limit` (`ErrPlanLimit`). `Meta(ctx)` says in
`Medicines` how many medicines your key reads.

## Images

`ProductImage` is the address of a medicine's picture. When it is under `/v1/images/`, Oushodh keeps the copy and
serves it with no key, so an `<img>` tag can show it. The file never changes, so it can be cached for a year.

`GetImage` takes the file name, not the whole address:

```go
if medicine.ProductImage != nil && strings.HasPrefix(*medicine.ProductImage, client.ImageURL("")) {
	file := path.Base(*medicine.ProductImage)
	jpeg, err := client.GetImage(ctx, file)
	// ...
}
```

Any other address belongs to the source, and it can move or be removed.

## Develop

```sh
go vet ./...
gofmt -l .
go test -race ./...
```

The tests use `net/http/httptest`. They never call the API.

## License

MIT. See [LICENSE](LICENSE).
