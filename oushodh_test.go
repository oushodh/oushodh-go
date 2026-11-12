package oushodh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "oush_test_123"

const medicineJSON = `{
	"id": "MED-012933",
	"uuid": "01a1ba6f-f4d6-7143-ac84-9376f520eef8",
	"brand": "Napa",
	"category": null,
	"product_name": "Napa 500 mg Tablet",
	"product_image": null,
	"images": [],
	"unit": "Strip",
	"size": "10 Tablets",
	"price": 12.5,
	"price_per": "strip",
	"price_source": "dgda_price",
	"unit_prices": [],
	"prices": {
		"currency": "BDT",
		"unit": {"amount": 1.25, "source": "dgda_price"},
		"sizes": [],
		"strip": {"amount": 12.5, "units": 10, "source": "dgda_price", "new_part": 1},
		"packs": [{"amount": 450, "units": null, "label": "30's pack", "source": "dgda_price"}]
	},
	"description_en": null,
	"description_bn": null,
	"dosage_form": "Tablet",
	"strength": "500 mg",
	"generic_name": "Paracetamol",
	"manufacturer": null,
	"source_url": null,
	"last_updated": "2026-11-09T15:27:34Z",
	"dgda_listed": true,
	"medicine_type": "allopathic",
	"new_field": {"a": 1}
}`

func newTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := NewClient(append([]Option{WithAPIKey(testKey), WithBaseURL(server.URL + "/")}, opts...)...)

	if err != nil {
		t.Fatal(err)
	}

	return client
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"code":%q,"message":"something about %s"}}`, code, code)
}

func TestNewClientNeedsKey(t *testing.T) {
	t.Setenv("OUSHODH_API_KEY", "")

	if _, err := NewClient(); err == nil {
		t.Fatal("want an error with no key")
	}

	t.Setenv("OUSHODH_API_KEY", testKey)

	client, err := NewClient()

	if err != nil {
		t.Fatal(err)
	}

	if client.apiKey != testKey {
		t.Fatal("want the key from OUSHODH_API_KEY")
	}
}

func TestMetaSendsKeyAndUserAgent(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta" {
			t.Errorf("path = %q", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer "+testKey {
			t.Errorf("Authorization = %q", got)
		}

		if got := r.Header.Get("User-Agent"); got != "oushodh-go/"+Version {
			t.Errorf("User-Agent = %q", got)
		}

		fmt.Fprint(w, `{"client":"acme","id_profile":"med","medicines":58937,"last_import":"2026-11-09T15:27:40Z"}`)
	})

	meta, err := client.Meta(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	if meta.Client != "acme" || meta.IDProfile != "med" || meta.Medicines != 58937 {
		t.Fatalf("meta = %+v", meta)
	}

	if meta.LastImport == nil || !meta.LastImport.Equal(time.Date(2026, 11, 9, 15, 27, 40, 0, time.UTC)) {
		t.Fatalf("last_import = %v", meta.LastImport)
	}
}

func TestListMedicinesQuery(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		want := "cursor=abc&limit=500&q=napa+500&updated_since=2026-11-08T09%3A30%3A00Z"

		if r.URL.Path != "/v1/medicines" || r.URL.RawQuery != want {
			t.Errorf("url = %s?%s", r.URL.Path, r.URL.RawQuery)
		}

		fmt.Fprintf(w, `{"items":[%s],"next":null,"total":1}`, medicineJSON)
	})

	// A time with a zone and a fraction goes out in UTC, to the second.
	since := time.Date(2026, 11, 8, 15, 30, 0, 999, time.FixedZone("BST", 6*3600))
	page, err := client.ListMedicines(context.Background(), ListParams{Limit: 500, Cursor: "abc", UpdatedSince: since, Q: "napa 500"})

	if err != nil {
		t.Fatal(err)
	}

	if page.Total != 1 || page.Next != nil || len(page.Items) != 1 {
		t.Fatalf("page = %+v", page)
	}

	m := page.Items[0]

	if m.ID != "MED-012933" || m.Price == nil || *m.Price != 12.5 || m.MedicineType == nil || *m.MedicineType != Allopathic {
		t.Fatalf("medicine = %+v", m)
	}

	if m.Prices.Strip == nil || m.Prices.Strip.Units == nil || *m.Prices.Strip.Units != 10 || m.Prices.Packs[0].Units != nil {
		t.Fatalf("prices = %+v", m.Prices)
	}

	if string(m.Extra["new_field"]) != `{"a": 1}` || len(m.Extra) != 1 {
		t.Fatalf("extra = %v", m.Extra)
	}

	if string(m.Prices.Strip.Extra["new_part"]) != "1" {
		t.Fatalf("strip extra = %v", m.Prices.Strip.Extra)
	}

	if m.Prices.Extra != nil {
		t.Fatalf("prices extra = %v", m.Prices.Extra)
	}
}

func TestMedicinesPagesToTheEnd(t *testing.T) {
	var cursors []string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)

		switch cursor {
		case "":
			fmt.Fprint(w, `{"items":[{"id":"1"},{"id":"2"}],"next":"p2","total":3}`)
		case "p2":
			fmt.Fprint(w, `{"items":[{"id":"3"}],"next":null,"total":3}`)
		default:
			t.Errorf("cursor = %q", cursor)
		}
	})

	var ids []string

	for medicine, err := range client.Medicines(context.Background(), ListParams{Limit: 2}) {
		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, medicine.ID)
	}

	if strings.Join(ids, ",") != "1,2,3" || strings.Join(cursors, ",") != ",p2" {
		t.Fatalf("ids = %v, cursors = %v", ids, cursors)
	}
}

func TestMedicinesStopsOnBreakAndError(t *testing.T) {
	var calls atomic.Int32

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			writeError(w, 400, "bad_request")

			return
		}

		fmt.Fprint(w, `{"items":[{"id":"1"},{"id":"2"}],"next":"p2","total":4}`)
	})

	for range client.Medicines(context.Background(), ListParams{}) {
		break
	}

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}

	var last error

	for _, err := range client.Medicines(context.Background(), ListParams{}) {
		last = err
	}

	if !errors.Is(last, ErrBadRequest) {
		t.Fatalf("err = %v", last)
	}
}

func TestSearchQuery(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/medicines/search" || r.URL.RawQuery != "limit=5&q=napa" {
			t.Errorf("url = %s?%s", r.URL.Path, r.URL.RawQuery)
		}

		fmt.Fprintf(w, `{"items":[%s],"total":42}`, medicineJSON)
	})

	result, err := client.Search(context.Background(), "napa", 5)

	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 42 || result.Items[0].Brand != "Napa" {
		t.Fatalf("result = %+v", result)
	}
}

func TestGetMedicineEscapesRefAndReadsMonograph(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1/medicines/a%2Fb%20c" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}

		body := strings.Replace(medicineJSON, `"brand"`, `"monograph":{"source":"example_source","indications":"Fever","composition":null,
			"pharmacology":null,"dosage":null,"interactions":null,"contraindications":null,"side_effects":null,
			"pregnancy_lactation":null,"precautions":null,"overdose":null,"storage":null},"brand"`, 1)
		fmt.Fprint(w, body)
	})

	detail, err := client.GetMedicine(context.Background(), "a/b c")

	if err != nil {
		t.Fatal(err)
	}

	if detail.ID != "MED-012933" || detail.Monograph == nil || *detail.Monograph.Indications != "Fever" {
		t.Fatalf("detail = %+v", detail)
	}

	if _, ok := detail.Extra["monograph"]; ok || len(detail.Extra) != 1 {
		t.Fatalf("extra = %v", detail.Extra)
	}
}

func TestGetMedicineNullMonograph(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"MED-1","monograph":null}`)
	})

	detail, err := client.GetMedicine(context.Background(), "MED-1")

	if err != nil {
		t.Fatal(err)
	}

	if detail.Monograph != nil || detail.Extra != nil {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestImagesWithoutKey(t *testing.T) {
	file := strings.Repeat("a", 64) + ".jpg"

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/"+file {
			t.Errorf("path = %q", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "" {
			t.Error("the image request sent a key")
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte{0xff, 0xd8, 0xff})
	})

	if !strings.HasSuffix(client.ImageURL(file), "/v1/images/"+file) || strings.Contains(client.ImageURL(file), "//v1") {
		t.Fatalf("ImageURL = %q", client.ImageURL(file))
	}

	jpeg, err := client.GetImage(context.Background(), file)

	if err != nil {
		t.Fatal(err)
	}

	if string(jpeg) != "\xff\xd8\xff" {
		t.Fatalf("bytes = %x", jpeg)
	}
}

func TestErrorFamilies(t *testing.T) {
	cases := []struct {
		status int
		code   string
		want   []error
	}{
		{400, "bad_request", []error{ErrBadRequest}},
		{401, "unauthorized", []error{ErrUnauthorized}},
		{403, "plan_expired", []error{ErrForbidden, ErrPlanExpired}},
		{403, "plan_limit", []error{ErrForbidden, ErrPlanLimit}},
		{404, "not_found", []error{ErrNotFound}},
		{429, "quota_exceeded", []error{ErrRateLimited, ErrQuotaExceeded}},
		{500, "internal", []error{ErrServer}},
	}
	all := []error{ErrBadRequest, ErrUnauthorized, ErrForbidden, ErrPlanExpired, ErrPlanLimit, ErrNotFound,
		ErrRateLimited, ErrQuotaExceeded, ErrServer, ErrNetwork}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				writeError(w, tc.status, tc.code)
			}, WithMaxRetries(0))

			_, err := client.GetMedicine(context.Background(), "MED-1")

			var e *Error

			if !errors.As(err, &e) || e.Status != tc.status || e.Code != tc.code || e.Message != "something about "+tc.code {
				t.Fatalf("err = %#v", err)
			}

			for _, sentinel := range all {
				want := false

				for _, w := range tc.want {
					want = want || w == sentinel
				}

				if errors.Is(err, sentinel) != want {
					t.Errorf("errors.Is(%v) = %v, want %v", sentinel, !want, want)
				}
			}

			if strings.Contains(err.Error(), testKey) {
				t.Fatalf("the key is in the error: %v", err)
			}
		})
	}
}

func TestNonJSONErrorBody(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<html>bad gateway</html>", http.StatusBadGateway)
	}, WithMaxRetries(0))

	_, err := client.Meta(context.Background())

	var e *Error

	if !errors.As(err, &e) || e.Status != 502 || e.Code != "" || e.Message != "HTTP 502" || !errors.Is(err, ErrServer) {
		t.Fatalf("err = %#v", err)
	}
}

func TestRateLimitedHonoursRetryAfter(t *testing.T) {
	var calls atomic.Int32

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			writeError(w, 429, "rate_limited")

			return
		}

		fmt.Fprint(w, `{"client":"acme","id_profile":"med","medicines":1,"last_import":null}`)
	})

	start := time.Now()

	if _, err := client.Meta(context.Background()); err != nil {
		t.Fatal(err)
	}

	if calls.Load() != 2 || time.Since(start) < time.Second {
		t.Fatalf("calls = %d after %v", calls.Load(), time.Since(start))
	}
}

func TestQuotaExceededReturnsAtOnce(t *testing.T) {
	var calls atomic.Int32

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "86400")
		writeError(w, 429, "quota_exceeded")
	})

	_, err := client.Meta(context.Background())

	var e *Error

	if !errors.As(err, &e) || e.RetryAfter != 24*time.Hour || calls.Load() != 1 {
		t.Fatalf("err = %#v, calls = %d", err, calls.Load())
	}
}

func TestServerErrorRetriedThenSuccess(t *testing.T) {
	var calls atomic.Int32

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			writeError(w, 500, "internal")

			return
		}

		fmt.Fprint(w, `{"items":[],"total":0}`)
	})

	if _, err := client.Search(context.Background(), "napa", 0); err != nil {
		t.Fatal(err)
	}

	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
}

func TestRetriesStopAtMax(t *testing.T) {
	var calls atomic.Int32

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeError(w, 503, "internal")
	}, WithMaxRetries(1))

	if _, err := client.Meta(context.Background()); !errors.Is(err, ErrServer) {
		t.Fatal(err)
	}

	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestNoRetryOn404(t *testing.T) {
	var calls atomic.Int32

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeError(w, 404, "not_found")
	})

	if _, err := client.GetMedicine(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestTimeoutWrapsDeadlineExceeded(t *testing.T) {
	block := make(chan struct{})

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}, WithTimeout(50*time.Millisecond), WithMaxRetries(0))

	defer close(block)

	_, err := client.Meta(context.Background())

	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v", err)
	}

	if strings.Contains(err.Error(), testKey) {
		t.Fatalf("the key is in the error: %v", err)
	}
}

func TestNetworkErrorRetriedAndKeyHidden(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	client, err := NewClient(WithAPIKey(testKey), WithBaseURL(server.URL), WithMaxRetries(1))

	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Meta(context.Background())

	var e *Error

	if !errors.As(err, &e) || e.Status != 0 || !errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %#v", err)
	}

	if strings.Contains(err.Error(), testKey) {
		t.Fatalf("the key is in the error: %v", err)
	}
}

func TestCanceledContextStopsRetries(t *testing.T) {
	var calls atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cancel()
		writeError(w, 500, "internal")
	})

	// The cancel can beat the 500 to the client, so either error is right.
	if _, err := client.Meta(ctx); !errors.Is(err, ErrServer) && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestBadJSONIsAnError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	})

	_, err := client.Meta(context.Background())

	var syntax *json.SyntaxError

	if !errors.As(err, &syntax) || errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v", err)
	}
}
