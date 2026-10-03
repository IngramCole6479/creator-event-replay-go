package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReplayDecision(t *testing.T) {
	for _, tc := range []struct {
		name, event, queue, history string
		wantCalls, wantStatus       int
	}{
		{"asset history", "digital_asset_delivery", "asset-delivery", `[{"id":"delivery-1"}]`, 1, 200},
		{"subscriber history", "subscriber_update", "subscriber-updates", `[{"id":"delivery-2"}]`, 1, 200},
		{"content history", "content_processing", "content-processing", `[{"id":"delivery-3"}]`, 1, 200},
		{"no history", "digital_asset_delivery", "asset-delivery", `[]`, 0, 200},
		{"wrong queue", "subscriber_update", "asset-delivery", `[]`, 0, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("missing bearer key")
				}
				if r.Method == http.MethodPost {
					calls++
					if r.Header.Get("Idempotency-Key") != "run-123" {
						t.Error("missing idempotency key")
					}
					w.Write([]byte(`{"ok":true,"data":{"accepted":true}}`))
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method %s", r.Method)
				}
				w.Write([]byte(`{"ok":true,"data":` + tc.history + `}`))
			}))
			defer upstream.Close()
			body := `{"webhook_id":"hook-1","event":"` + tc.event + `","queue":"` + tc.queue + `","request_id":"run-123"}`
			req := httptest.NewRequest(http.MethodPost, "/replay", strings.NewReader(body))
			res := httptest.NewRecorder()
			replayHandler(replayClient{httpClient: upstream.Client(), baseURL: upstream.URL, key: "test-key"}).ServeHTTP(res, req)
			if res.Code != tc.wantStatus || calls != tc.wantCalls {
				t.Fatalf("status=%d redrives=%d body=%s", res.Code, calls, res.Body.String())
			}
		})
	}
}

func TestEnvelopeRejectionStaysClientError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"ok":false,"error":{"code":"INVALID_ARGUMENT","message":"invalid selection"}}`))
	}))
	defer upstream.Close()
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/replay", strings.NewReader(`{"webhook_id":"hook-1","event":"subscriber_update","queue":"subscriber-updates","request_id":"run-123"}`))
	replayHandler(replayClient{httpClient: upstream.Client(), baseURL: upstream.URL, key: "test-key"}).ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", res.Code)
	}
}
