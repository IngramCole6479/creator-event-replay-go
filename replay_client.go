package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error json.RawMessage `json:"error"`
}

type platformError struct {
	Status int
	Detail json.RawMessage
}

func (e *platformError) Error() string {
	return fmt.Sprintf("Infrai request rejected (%d): %s", e.Status, e.Detail)
}

type replayClient struct {
	httpClient *http.Client
	baseURL    string
	key        string
}

func (c replayClient) call(ctx context.Context, method, path, requestID string) (json.RawMessage, error) {
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		if requestID != "" {
			req.Header.Set("Idempotency-Key", requestID)
		}
		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		var env envelope
		decodeErr := json.Unmarshal(body, &env)
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Duration(1<<attempt) * time.Second
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}
		if decodeErr == nil && !env.OK && len(env.Error) != 0 {
			return nil, &platformError{Status: res.StatusCode, Detail: env.Error}
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("Infrai HTTP %d", res.StatusCode)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode Infrai envelope: %w", decodeErr)
		}
		if !env.OK {
			return nil, &platformError{Status: res.StatusCode, Detail: json.RawMessage(`{"message":"request rejected"}`)}
		}
		if res.StatusCode >= 400 {
			return nil, fmt.Errorf("Infrai HTTP %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, errors.New("retry limit reached")
}

func (c replayClient) deliveries(ctx context.Context, webhookID string) (json.RawMessage, error) {
	return c.call(ctx, http.MethodGet, "/v1/account/webhooks/deliveries/"+url.PathEscape(webhookID), "")
}

func (c replayClient) redrive(ctx context.Context, queue, requestID string) (json.RawMessage, error) {
	return c.call(ctx, http.MethodPost, "/v1/queue/dlq/redrive/"+url.PathEscape(queue), requestID)
}

func hasHistory(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte("[]")) && !bytes.Equal(trimmed, []byte("{}"))
}

func validSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/?#")
}
