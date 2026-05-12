// Package sms wraps the Twilio Programmable Messaging REST API.
//
// We intentionally don't pull in the official Twilio SDK — the API surface
// we use is small (one POST to send, one HMAC check to verify webhooks) and
// avoiding a heavy SDK keeps dependencies minimal.
package sms

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client talks to Twilio. Construct one per process and reuse it — the
// underlying http.Client pools connections.
type Client struct {
	AccountSID string
	AuthToken  string
	From       string // Twilio phone number in E.164 (e.g. +17065551234)
	httpClient *http.Client

	// DryRun, if true, logs the message but doesn't actually call Twilio.
	// Useful for local dev without a real Twilio account.
	DryRun bool
}

// NewClient returns a Client. If accountSID or authToken is empty, the client
// will operate in DryRun mode — sends are logged but not transmitted. This
// makes local development possible without Twilio credentials.
func NewClient(accountSID, authToken, from string) *Client {
	c := &Client{
		AccountSID: accountSID,
		AuthToken:  authToken,
		From:       from,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	if accountSID == "" || authToken == "" || from == "" {
		c.DryRun = true
	}
	return c
}

// SendResult is what we get back from a Send call.
type SendResult struct {
	SID    string
	Status string
}

// Send dispatches one SMS message. Returns the Twilio SID and status string on
// success. In DryRun mode, returns ("dryrun-…", "queued").
func (c *Client) Send(ctx context.Context, to, body string) (*SendResult, error) {
	if to == "" || body == "" {
		return nil, errors.New("send: to and body are required")
	}
	if c.DryRun {
		fmt.Printf("[SMS DRYRUN] to=%s body=%q\n", to, body)
		return &SendResult{
			SID:    fmt.Sprintf("dryrun-%d", time.Now().UnixNano()),
			Status: "queued",
		}, nil
	}

	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", c.AccountSID)
	form := url.Values{}
	form.Set("From", c.From)
	form.Set("To", to)
	form.Set("Body", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.AccountSID, c.AuthToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twilio request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Twilio returns 201 Created on success, 4xx for client errors with a
	// JSON {code, message} body we surface to the caller.
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("twilio %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("twilio response parse: %w", err)
	}
	return &SendResult{SID: parsed.SID, Status: parsed.Status}, nil
}

// VerifyWebhook validates a request from Twilio per
// https://www.twilio.com/docs/usage/security#validating-requests.
//
// Twilio HMAC-SHA1's (request URL + sorted concatenated POST params) using
// the AuthToken as the key, base64-encodes the result, and puts it in the
// X-Twilio-Signature header. We recompute and compare.
//
// The url argument must be the FULL request URL that Twilio called, including
// scheme, host, port, path, and query. If your server sits behind a proxy
// that strips/changes the host, configure the proxy to set X-Forwarded-Host
// and pass the reconstructed URL in here.
func (c *Client) VerifyWebhook(signature, fullURL string, form url.Values) bool {
	if c.AuthToken == "" {
		// In DryRun / unconfigured mode we accept all webhooks — this lets
		// you test the inbound handler locally without Twilio at all.
		return true
	}
	if signature == "" {
		return false
	}

	// Sort the form keys, concatenate key+value pairs, append to URL.
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(fullURL)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(form.Get(k))
	}

	mac := hmac.New(sha1.New, []byte(c.AuthToken))
	mac.Write([]byte(sb.String()))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
