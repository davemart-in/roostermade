package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrPrivacyUnavailable = errors.New("privacy api unavailable: missing or invalid api key")

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type CreateCardInput struct {
	Memo          string   `json:"memo,omitempty"`
	SpendLimit    int64    `json:"spend_limit,omitempty"`
	SpendLimitDur string   `json:"spend_limit_duration,omitempty"`
	State         string   `json:"state,omitempty"`
	MerchantLock  []string `json:"merchant_lock,omitempty"`
	CategoryLock  []string `json:"category_lock,omitempty"`
}

type Card struct {
	Token string `json:"token"`
	State string `json:"state"`
	Pan   string `json:"pan"`
	Cvv   string `json:"cvv"`
	ExpM  int    `json:"exp_month"`
	ExpY  int    `json:"exp_year"`
	Last4 string `json:"last_four"`
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) Available() bool {
	return c.apiKey != ""
}

func (c *Client) CreateCard(ctx context.Context, in CreateCardInput) (*Card, error) {
	if !c.Available() {
		return nil, ErrPrivacyUnavailable
	}
	var out Card
	if err := c.do(ctx, http.MethodPost, "/card", in, &out); err == nil && out.Token != "" {
		return &out, nil
	}
	if err := c.do(ctx, http.MethodPost, "/cards", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateCard(ctx context.Context, token string, payload map[string]any) error {
	if !c.Available() {
		return ErrPrivacyUnavailable
	}
	if err := c.do(ctx, http.MethodPut, "/card/"+token, payload, nil); err == nil {
		return nil
	}
	return c.do(ctx, http.MethodPatch, "/cards/"+token, payload, nil)
}

func (c *Client) SetCardState(ctx context.Context, token, state string) error {
	payload := map[string]any{"state": strings.ToUpper(state)}
	return c.UpdateCard(ctx, token, payload)
}

func (c *Client) do(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "api-key "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("privacy api %s %s failed: status=%d body=%s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
