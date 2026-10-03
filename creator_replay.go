package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

type replayRequest struct {
	WebhookID string `json:"webhook_id"`
	Queue     string `json:"queue"`
	Event     string `json:"event"`
	RequestID string `json:"request_id"`
}

func queueForEvent(event string) string {
	switch event {
	case "digital_asset_delivery":
		return "asset-delivery"
	case "subscriber_update":
		return "subscriber-updates"
	case "content_processing":
		return "content-processing"
	default:
		return ""
	}
}

func replayHandler(client replayClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var input replayRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !validSegment(input.WebhookID) || !validSegment(input.RequestID) || input.Queue == "" || input.Queue != queueForEvent(input.Event) {
			http.Error(w, "invalid replay selection", http.StatusBadRequest)
			return
		}
		history, err := client.deliveries(r.Context(), input.WebhookID)
		if err != nil {
			respondError(w, err)
			return
		}
		result := struct {
			Event      string          `json:"event"`
			Replayed   bool            `json:"replayed"`
			Deliveries json.RawMessage `json:"deliveries"`
			Redrive    json.RawMessage `json:"redrive,omitempty"`
		}{Event: input.Event, Deliveries: history}
		if hasHistory(history) {
			result.Redrive, err = client.redrive(r.Context(), input.Queue, input.RequestID)
			if err != nil {
				respondError(w, err)
				return
			}
			result.Replayed = true
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
}

func respondError(w http.ResponseWriter, err error) {
	var apiErr *platformError
	if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
		http.Error(w, apiErr.Error(), apiErr.Status)
		return
	}
	log.Printf("replay: %v", err)
	http.Error(w, "upstream request failed", http.StatusBadGateway)
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("set INFRAI_API_KEY")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	client := replayClient{httpClient: &http.Client{Timeout: 15 * time.Second}, baseURL: defaultBaseURL, key: key}
	log.Fatal(http.ListenAndServe(addr, replayHandler(client)))
}
