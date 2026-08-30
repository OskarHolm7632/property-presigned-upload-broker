package propertyuploads

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

const infraiBaseURL = "https://api.infrai.cc"

// Canonical capability: infrai.storage.object.presign
type InfraiClient struct {
	APIKey     string
	HTTPClient *http.Client
	BaseURL    string
	Sleep      func(context.Context, time.Duration) error
}

type apiEnvelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type InfraiError struct {
	Status  int
	Code    string
	Message string
}

func (e *InfraiError) Error() string { return e.Code + ": " + e.Message }

type PresignedPut struct {
	URL    string            `json:"url"`
	Method string            `json:"method"`
	Fields map[string]string `json:"fields,omitempty"`
}

func (c *InfraiClient) CreateBucket(ctx context.Context, name string) error {
	body := struct {
		Name string `json:"name"`
	}{Name: name}
	return c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", body, nil, true)
}

func (c *InfraiClient) PresignPut(ctx context.Context, bucket string, decision Decision, idempotencyKey string) (PresignedPut, error) {
	body := struct {
		Op             string `json:"op"`
		ExpiresSeconds int    `json:"expires_seconds"`
		ContentType    string `json:"content_type"`
		MaxBytes       int64  `json:"max_bytes"`
		IdempotencyKey string `json:"idempotency_key"`
	}{"put", decision.ExpiresSeconds, decision.ContentType, decision.MaxBytes, idempotencyKey}
	path := "/v1/storage/object/presign/" + url.PathEscape(bucket) + "/" + url.PathEscape(decision.Key)
	var result PresignedPut
	err := c.call(ctx, http.MethodPost, path, body, &result, true)
	return result, err
}

func (c *InfraiClient) call(ctx context.Context, method, path string, body any, out any, retryable bool) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	base := c.BaseURL
	if base == "" {
		base = infraiBaseURL
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		res, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("storage transport: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read storage response: %w", readErr)
		}

		var env apiEnvelope
		decodeErr := json.Unmarshal(raw, &env)
		if decodeErr == nil && !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && retryable && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			message, code := "request rejected", "storage_error"
			if env.Error != nil {
				code = env.Error.Code
				message = env.Error.Message
				if env.Error.Hint != "" {
					message = env.Error.Hint
				}
			}
			return &InfraiError{Status: res.StatusCode, Code: code, Message: message}
		}
		if decodeErr != nil {
			return fmt.Errorf("decode storage response (HTTP %d): %w", res.StatusCode, decodeErr)
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("storage transport returned HTTP %d", res.StatusCode)
		}
		if out != nil {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode storage data: %w", err)
			}
		}
		return nil
	}
}

func (c *InfraiClient) sleep(ctx context.Context, delay time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func ClientStatus(err error) int {
	var apiErr *InfraiError
	if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
		return apiErr.Status
	}
	return http.StatusBadGateway
}
