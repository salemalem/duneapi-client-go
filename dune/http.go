package dune

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

var ErrorReqUnsuccessful = errors.New("request was not successful")

type ErrorResponse struct {
	Error string `json:"error"`
}

type RateLimit struct {
	Limit     int
	Remaining int
	Reset     int64
}

type APIError struct {
	StatusCode  int
	StatusText  string
	BodySnippet string
	RateLimit   *RateLimit
	RetryAfter  time.Duration
}

func (e *APIError) Error() string {
	if e.BodySnippet != "" {
		return fmt.Sprintf("http %d %s: %s", e.StatusCode, e.StatusText, e.BodySnippet)
	}
	return fmt.Sprintf("http %d %s", e.StatusCode, e.StatusText)
}


func parseRateLimitHeaders(h http.Header) *RateLimit {
	limStr := h.Get("X-RateLimit-Limit")
	remStr := h.Get("X-RateLimit-Remaining")
	resetStr := h.Get("X-RateLimit-Reset")

	var limit, remaining int
	var reset int64

	if limStr != "" {
		if v, err := strconv.Atoi(limStr); err == nil {
			limit = v
		}
	}
	if remStr != "" {
		if v, err := strconv.Atoi(remStr); err == nil {
			remaining = v
		}
	}
	if resetStr != "" {
		if v, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			reset = v
		}
	}

	if limit == 0 && remaining == 0 && reset == 0 {
		return nil
	}
	return &RateLimit{Limit: limit, Remaining: remaining, Reset: reset}
}


func decodeBody(resp *http.Response, dest interface{}) error {
	defer resp.Body.Close()
	err := json.NewDecoder(resp.Body).Decode(dest)
	if err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	return nil
}

func httpRequest(apiKey string, req *http.Request) (*http.Response, error) {
	req.Header.Add("X-DUNE-API-KEY", apiKey)
	p := defaultRetryPolicy
	attempt := 1
	for {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if attempt >= p.MaxAttempts {
				return nil, fmt.Errorf("failed to send request: %w", err)
			}
			time.Sleep(p.NextBackoff(attempt))
			attempt++
			continue
		}

		if resp.StatusCode == 200 {
			return resp, nil
		}

		defer resp.Body.Close()
		snippetBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		var errorResp ErrorResponse
		if err := json.Unmarshal(snippetBytes, &errorResp); err == nil && errorResp.Error != "" {
			msg := errorResp.Error
			rl := parseRateLimitHeaders(resp.Header)
			retryAfter := time.Duration(0)
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					retryAfter = time.Duration(secs) * time.Second
				}
			}
			apiErr := &APIError{StatusCode: resp.StatusCode, StatusText: resp.Status, BodySnippet: msg, RateLimit: rl, RetryAfter: retryAfter}
			retryable := false
			for _, code := range p.RetryableStatusCodes {
				if resp.StatusCode == code {
					retryable = true
					break
				}
			}
			if retryable && attempt < p.MaxAttempts {
				sleep := p.NextBackoff(attempt)
				if apiErr.RetryAfter > 0 && apiErr.RetryAfter > sleep {
					sleep = apiErr.RetryAfter
				}
				time.Sleep(sleep)
				attempt++
				continue
			}
			return nil, fmt.Errorf("%w: %v", ErrorReqUnsuccessful, apiErr)
		} else {
			msg := string(snippetBytes)
			rl := parseRateLimitHeaders(resp.Header)
			retryAfter := time.Duration(0)
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					retryAfter = time.Duration(secs) * time.Second
				}
			}
			apiErr := &APIError{StatusCode: resp.StatusCode, StatusText: resp.Status, BodySnippet: msg, RateLimit: rl, RetryAfter: retryAfter}
			retryable := false
			for _, code := range p.RetryableStatusCodes {
				if resp.StatusCode == code {
					retryable = true
					break
				}
			}
			if retryable && attempt < p.MaxAttempts {
				sleep := p.NextBackoff(attempt)
				if apiErr.RetryAfter > 0 && apiErr.RetryAfter > sleep {
					sleep = apiErr.RetryAfter
				}
				time.Sleep(sleep)
				attempt++
				continue
			}
			return nil, fmt.Errorf("%w: %v", ErrorReqUnsuccessful, apiErr)
		}
		rl := parseRateLimitHeaders(resp.Header)
		retryAfter := time.Duration(0)
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		apiErr := &APIError{
			StatusCode:  resp.StatusCode,
			StatusText: resp.Status,
			BodySnippet: msg,
			RateLimit:  rl,
			RetryAfter: retryAfter,
		}
		retryable := false
		for _, code := range p.RetryableStatusCodes {
			if resp.StatusCode == code {
				retryable = true
				break
			}
		}
		// unreachable due to early returns above; kept for clarity
		return nil, fmt.Errorf("%w: unexpected error state", ErrorReqUnsuccessful)
	}
}
