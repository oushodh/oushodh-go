# Changelog

## 0.1.0 (unreleased)

- First version of the Go SDK for the Oushodh API `/v1`.
- `NewClient` with `WithAPIKey`, `WithBaseURL`, `WithHTTPClient`, `WithTimeout` and `WithMaxRetries`.
- `Meta`, `ListMedicines`, `Medicines` (an `iter.Seq2` that pages to the end), `Search`, `GetMedicine`, `ImageURL` and
  `GetImage`. Every call takes a `context.Context`.
- Typed models for `Medicine`, `MedicineDetail`, `Monograph`, `Page`, `SearchResult` and `Meta`. Unknown fields are
  kept in `Extra`.
- One `*Error` type with `Status`, `Code`, `Message` and `RetryAfter`, and sentinel errors for `errors.Is`.
- Retries on `429 rate_limited` (honours `Retry-After`), `5xx` and network errors.
