package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	tokenEndpoint    = "https://auth.openai.com/api/accounts/oauth/token"
	revocationConfig = "https://auth.openai.com/.well-known/openid-configuration"
	resource         = "https://api.openai.com/v1"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Manager struct {
	path        string
	staticToken string
	client      HTTPDoer
	mu          sync.Mutex
}

func NewManager(path, staticToken string, client HTTPDoer) *Manager {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Manager{path: path, staticToken: staticToken, client: client}
}

func (m *Manager) Token(ctx context.Context) (string, error) {
	if m.staticToken != "" {
		return m.staticToken, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	credential, err := LoadCredential(m.path)
	if err != nil {
		return "", err
	}
	if time.Now().Add(60 * time.Second).Before(credential.ExpiresAt()) {
		return credential.AccessToken, nil
	}
	if credential.RefreshToken == "" {
		return "", fmt.Errorf("access token expired and no refresh token is available: %w", ErrNotLoggedIn)
	}
	credential, err = refresh(ctx, m.client, credential)
	if err != nil {
		return "", err
	}
	if err := SaveCredential(m.path, credential); err != nil {
		return "", err
	}
	return credential.AccessToken, nil
}

func refresh(ctx context.Context, client HTTPDoer, credential Credential) (Credential, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {credential.ClientID},
		"refresh_token": {credential.RefreshToken},
		"resource":      {resource},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Credential{}, fmt.Errorf("create refresh request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return Credential{}, fmt.Errorf("refresh access token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return Credential{}, fmt.Errorf("refresh access token: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var token tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return Credential{}, fmt.Errorf("decode refreshed token: %w", err)
	}
	if token.AccessToken == "" {
		return Credential{}, fmt.Errorf("refresh response did not contain an access token")
	}
	credential.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		credential.RefreshToken = token.RefreshToken
	}
	if token.IDToken != "" {
		credential.IDToken = token.IDToken
	}
	if token.TokenType != "" {
		credential.TokenType = token.TokenType
	}
	if token.ExpiresIn > 0 {
		credential.ExpiresIn = token.ExpiresIn
	}
	if token.Scope != "" {
		credential.Scopes = strings.Fields(token.Scope)
	}
	credential.SavedAt = time.Now().UTC()
	return credential, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}
