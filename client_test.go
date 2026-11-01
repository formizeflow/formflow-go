package formflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, h http.HandlerFunc) (*Client, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var slept []time.Duration
	c, err := New(Options{APIKey: "ff_test", BaseURL: srv.URL, Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return c, &slept
}

func TestHeadersAndEnvelope(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ff_test" || r.Header.Get("FormFlow-Version") != DefaultVersion || r.URL.Path != "/api/v1/forms/frm_1" {
			t.Errorf("bad request: %v %s", r.Header, r.URL.Path)
		}
		w.Write([]byte(`{"data":{"id":"frm_1","name":"x"}}`))
	})
	f, err := c.GetForm(context.Background(), "frm_1")
	if err != nil || f.ID != "frm_1" {
		t.Fatalf("%v %v", f, err)
	}
}

func TestRetries429WithRetryAfterAndReusesIdempotencyKey(t *testing.T) {
	var keys []string
	var n int32
	c, slept := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if atomic.AddInt32(&n, 1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			w.Write([]byte(`{"error":{"code":"rate_limited","message":"slow"}}`))
			return
		}
		w.Write([]byte(`{"data":{}}`))
	})
	if _, err := c.PublishForm(context.Background(), "frm_1"); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("keys %v", keys)
	}
	if len(*slept) != 1 || (*slept)[0] != 2*time.Second {
		t.Fatalf("slept %v", *slept)
	}
}

func TestTypedErrorWithoutRetryOn4xx(t *testing.T) {
	var n int32
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.Header().Set("X-Request-Id", "req_1")
		w.WriteHeader(422)
		w.Write([]byte(`{"error":{"code":"validation_failed","message":"bad","details":{"issues":[]}}}`))
	})
	_, err := c.PublishForm(context.Background(), "frm_1")
	e, ok := err.(*Error)
	if !ok || e.Status != 422 || e.Code != "validation_failed" || e.RequestID != "req_1" || n != 1 {
		t.Fatalf("%#v n=%d", err, n)
	}
}

func TestEachPaginates(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			w.Write([]byte(`{"data":[{"id":"sub_1"}],"nextCursor":"c2"}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"sub_2"}],"nextCursor":null}`))
	})
	var ids []string
	err := c.EachSubmission(context.Background(), "frm_1", url.Values{"status": {"completed"}}, func(s Submission) error { ids = append(ids, s.ID); return nil })
	if err != nil || len(ids) != 2 || ids[1] != "sub_2" {
		t.Fatalf("%v %v", ids, err)
	}
}

const (
	whBody = `{"id":"evt_1","type":"form.published","created":1,"workspace_id":"ws","data":{}}`
	// computed independently with node:crypto using the server's scheme: HMAC-SHA256("whsec_test", "1700000000." + body)
	whVector = "t=1700000000,v1=1797019d2af87623b436a19f93ebbf5be4717500a701adf21375c87b8335d38b"
)

func TestSignatureMatchesServerVector(t *testing.T) {
	if got := SignPayload("whsec_test", []byte(whBody), 1700000000); got != whVector {
		t.Fatalf("got %s", got)
	}
	now := time.Unix(1700000100, 0)
	if !VerifySignature("whsec_test", []byte(whBody), whVector, 0, now) {
		t.Fatal("valid signature rejected")
	}
	for name, ok := range map[string]bool{
		"tampered":     VerifySignature("whsec_test", []byte(whBody+" "), whVector, 0, now),
		"wrong secret": VerifySignature("other", []byte(whBody), whVector, 0, now),
		"stale":        VerifySignature("whsec_test", []byte(whBody), whVector, 0, time.Unix(1700001000, 0)),
		"junk":         VerifySignature("whsec_test", []byte(whBody), "garbage", 0, now),
		"empty":        VerifySignature("whsec_test", []byte(whBody), "", 0, now),
	} {
		if ok {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestRotatedSecretHeader(t *testing.T) {
	header := "t=1700000000,v1=" + string(make([]byte, 0)) + "00,v1=1797019d2af87623b436a19f93ebbf5be4717500a701adf21375c87b8335d38b"
	if !VerifySignature("whsec_test", []byte(whBody), header, 0, time.Unix(1700000000, 0)) {
		t.Fatal("second v1 value should match")
	}
}

func TestConstructEvent(t *testing.T) {
	h := SignPayload("s", []byte(whBody), time.Now().Unix())
	e, err := ConstructEvent("s", []byte(whBody), h)
	if err != nil || e.Type != "form.published" {
		t.Fatalf("%v %v", e, err)
	}
	if _, err := ConstructEvent("s", []byte(whBody), "t=1,v1=00"); err == nil {
		t.Fatal("expected error")
	}
}
