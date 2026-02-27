package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OpenAIClient struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	RedirectURI  string
	Scopes       []string
	HTTPClient   *http.Client
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func (c *OpenAIClient) BuildAuthURL(state string) (string, error) {
	if c.ClientID == "" || c.AuthURL == "" || c.RedirectURI == "" {
		return "", fmt.Errorf("openai oauth config missing")
	}
	u, err := url.Parse(c.AuthURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(c.Scopes, " "))
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *OpenAIClient) ExchangeCode(ctx context.Context, code string) (TokenResponse, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", c.RedirectURI)
	values.Set("client_id", c.ClientID)
	values.Set("client_secret", c.ClientSecret)
	return c.requestToken(ctx, values)
}

func (c *OpenAIClient) Refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)
	values.Set("client_id", c.ClientID)
	values.Set("client_secret", c.ClientSecret)
	return c.requestToken(ctx, values)
}

func (c *OpenAIClient) requestToken(ctx context.Context, values url.Values) (TokenResponse, error) {
	if c.TokenURL == "" {
		return TokenResponse{}, fmt.Errorf("openai token url missing")
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return TokenResponse{}, fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return TokenResponse{}, err
	}
	if tokenResp.AccessToken == "" {
		return TokenResponse{}, fmt.Errorf("token response missing access_token")
	}
	if tokenResp.ExpiresIn <= 0 {
		tokenResp.ExpiresIn = int64((1 * time.Hour).Seconds())
	}
	return tokenResp, nil
}
