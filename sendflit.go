// Package sendflit is the official SendFlit SDK for Go.
// Transactional email API for applications and AI agents.
//
//	sfl := sendflit.New("re_...")
//	res, err := sfl.Send(ctx, sendflit.Email{
//	    From: "you@yourdomain.com", To: "user@example.com",
//	    Subject: "Confirm your email", HTML: "<p>Confirm…</p>",
//	})
package sendflit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL   = "https://api.sendflit.com"
	Version          = "1.0.0"
	defaultTimeout   = 15 * time.Second
	defaultMaxRetry  = 2
	maxBatch         = 100
	userAgent        = "sendflit-go/" + Version
)

// Client is a SendFlit API client. Safe for concurrent use.
type Client struct {
	APIKey    string
	BaseURL   string
	HTTP      *http.Client
	MaxRetries int
}

// New creates a client with sensible defaults.
func New(apiKey string) *Client {
	return &Client{
		APIKey:  apiKey,
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: defaultTimeout},
		MaxRetries: defaultMaxRetry,
	}
}

// NewWithBaseURL points the client at an alternate API root.
func NewWithBaseURL(apiKey, baseURL string) *Client {
	c := New(apiKey)
	c.BaseURL = strings.TrimRight(baseURL, "/")
	return c
}

// Email is a single outbound message.
type Email struct {
	From        string        `json:"from"`
	To          string        `json:"to"`
	ToName      string        `json:"to_name,omitempty"`
	CC          []string      `json:"cc,omitempty"`
	BCC         []string      `json:"bcc,omitempty"`
	Subject     string        `json:"subject,omitempty"`
	HTML        string        `json:"html,omitempty"`
	Text        string        `json:"text,omitempty"`
	ReplyTo     string        `json:"reply_to,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ScheduledAt string        `json:"scheduled_at,omitempty"` // ISO 8601
	TemplateID  string        `json:"template_id,omitempty"`
	TemplateName string       `json:"template_name,omitempty"`
	Variables   map[string]string `json:"variables,omitempty"`
	Attachments []Attachment  `json:"attachments,omitempty"`
}

// Attachment content is base64-encoded automatically in Attach / AttachBytes.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentB64  string `json:"content_b64"`
	ContentType string `json:"content_type"`
}

// Attach adds a string attachment (convenience).
func (e *Email) Attach(filename, content, contentType string) {
	if contentType == "" {
		contentType = "text/plain"
	}
	e.Attachments = append(e.Attachments, Attachment{
		Filename:    filename,
		ContentB64:  base64.StdEncoding.EncodeToString([]byte(content)),
		ContentType: contentType,
	})
}

// AttachBytes adds a binary attachment (convenience).
func (e *Email) AttachBytes(filename string, content []byte, contentType string) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	e.Attachments = append(e.Attachments, Attachment{
		Filename:    filename,
		ContentB64:  base64.StdEncoding.EncodeToString(content),
		ContentType: contentType,
	})
}

// SendResult is the response for a successful send.
type SendResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	To     string `json:"to,omitempty"`
}

// APIError carries the HTTP status and parsed body of a failed call.
type APIError struct {
	Status int
	Body   string
	Msg    string
}

func (e *APIError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("sendflit: API error %d: %s", e.Status, e.Body)
}

// Send dispatches one email.
func (c *Client) Send(ctx context.Context, email Email) (*SendResult, error) {
	var out SendResult
	err := c.do(ctx, http.MethodPost, "/v1/email", email, &out)
	return &out, err
}

// Batch dispatches up to 100 emails in one call.
func (c *Client) Batch(ctx context.Context, emails []Email) ([]SendResult, error) {
	if len(emails) == 0 {
		return nil, errors.New("sendflit: batch requires at least one email")
	}
	if len(emails) > maxBatch {
		return nil, fmt.Errorf("sendflit: batch accepts at most %d emails", maxBatch)
	}
	var out struct {
		Results []SendResult `json:"results"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/emails/batch", map[string]any{"emails": emails}, &out)
	return out.Results, err
}

// ListEmailsOptions filters ListEmails.
type ListEmailsOptions struct {
	Limit  int
	Offset int
	Status string
}

// EmailItem is one row in ListEmails.
type EmailItem struct {
	ID      string `json:"id"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Status  string `json:"status"`
}

// ListEmails pages through sent email.
func (c *Client) ListEmails(ctx context.Context, opt ListEmailsOptions) ([]EmailItem, error) {
	q := url.Values{}
	if opt.Limit > 0 {
		q.Set("limit", strconv.Itoa(opt.Limit))
	}
	if opt.Offset > 0 {
		q.Set("offset", strconv.Itoa(opt.Offset))
	}
	if opt.Status != "" {
		q.Set("status", opt.Status)
	}
	path := "/v1/emails"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out struct {
		Emails []EmailItem `json:"emails"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Emails, err
}

// Domain is a sending domain.
type Domain struct {
	ID   string `json:"id"`
	Name string `json:"domain"`
}

// AddDomain registers a sending domain and returns its DNS records.
func (c *Client) AddDomain(ctx context.Context, name string) (*Domain, error) {
	var out Domain
	err := c.do(ctx, http.MethodPost, "/v1/domains", map[string]string{"name": name}, &out)
	return &out, err
}

// ListDomains returns the account's domains.
func (c *Client) ListDomains(ctx context.Context) ([]Domain, error) {
	var out struct {
		Domains []Domain `json:"domains"`
	}
	err := c.do(ctx, http.MethodGet, "/v1/domains", nil, &out)
	return out.Domains, err
}

// VerifyDomain re-checks DNS for a domain.
func (c *Client) VerifyDomain(ctx context.Context, domainID string) error {
	return c.do(ctx, http.MethodPost, "/v1/domains/"+url.PathEscape(domainID)+"/verify", nil, nil)
}

// Contact is an audience contact.
type Contact struct {
	Email    string         `json:"email"`
	Name     string         `json:"name,omitempty"`
	Audience string         `json:"audience,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// AddContact upserts a contact.
func (c *Client) AddContact(ctx context.Context, ct Contact) error {
	return c.do(ctx, http.MethodPost, "/v1/contacts", ct, nil)
}

// Suppress blocks an address from receiving mail.
func (c *Client) Suppress(ctx context.Context, email, reason string) error {
	if reason == "" {
		reason = "unsubscribe"
	}
	return c.do(ctx, http.MethodPost, "/v1/suppressions",
		map[string]string{"email": email, "reason": reason}, nil)
}

// Health probes the API root (not /v1).
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("sendflit: health %d", res.StatusCode)
	}
	return nil
}

// do executes a request with retries on 429/5xx.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	attempt := 0
	for {
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("User-Agent", userAgent)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()

		if res.StatusCode >= 200 && res.StatusCode < 300 {
			if out == nil || len(raw) == 0 {
				return nil
			}
			return json.Unmarshal(raw, out)
		}

		if (res.StatusCode == 429 || res.StatusCode >= 500) && attempt < c.MaxRetries {
			attempt++
			delay := 300 * (1 << attempt) * time.Millisecond
			if ra := res.Header.Get("Retry-After"); ra != "" {
				if secs, perr := strconv.Atoi(ra); perr == nil && secs > 0 {
					delay = time.Duration(secs) * time.Second
				}
			}
			delay += time.Duration(rand.Intn(150)) * time.Millisecond
			select {
			case <-time.After(delay):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		apiErr := &APIError{Status: res.StatusCode, Body: string(raw)}
		var parsed struct {
			Detail json.RawMessage `json:"detail"`
		}
		if json.Unmarshal(raw, &parsed) == nil {
			var s string
			if json.Unmarshal(parsed.Detail, &s) == nil {
				apiErr.Msg = s
			} else {
				var fields []struct {
					Msg string `json:"msg"`
				}
				if json.Unmarshal(parsed.Detail, &fields) == nil && len(fields) > 0 {
					msgs := make([]string, 0, len(fields))
					for _, f := range fields {
						msgs = append(msgs, f.Msg)
					}
					apiErr.Msg = strings.Join(msgs, "; ")
				}
			}
		}
		return apiErr
	}
}

// VerifyWebhookSignature checks an X-Sendflit-Signature header value.
func VerifyWebhookSignature(secret string, rawBody []byte, signature string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// APIKeyFromEnv reads SENDFLIT_API_KEY.
func APIKeyFromEnv() string { return os.Getenv("SENDFLIT_API_KEY") }
