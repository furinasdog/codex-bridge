package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const authorizeEndpoint = "https://auth.openai.com/api/accounts/authorize"

type LoginOptions struct {
	CredentialPath string
	Timeout        time.Duration
	OpenBrowser    bool
	AppName        string
	HTTPClient     HTTPDoer
	PrintURL       func(string)
}

type callbackResult struct {
	Code     string
	State    string
	ClientID string
	Scope    string
	Error    string
	Message  string
}

func Login(ctx context.Context, options LoginOptions) (Credential, error) {
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Minute
	}
	if options.AppName == "" {
		options.AppName = "Codex Bridge"
	}
	if options.PrintURL == nil {
		options.PrintURL = func(value string) { fmt.Printf("Open this URL to continue:\n%s\n", value) }
	}

	hostID, err := loadOrCreateHostID(options.CredentialPath)
	if err != nil {
		return Credential{}, err
	}
	existing, existingErr := LoadCredential(options.CredentialPath)
	returning := existingErr == nil
	clientID := "dynamic_agent_client"
	if returning {
		clientID = existing.ClientID
	}

	state, err := randomURLSafe(32)
	if err != nil {
		return Credential{}, err
	}
	nonce, err := randomURLSafe(32)
	if err != nil {
		return Credential{}, err
	}
	verifier, err := randomURLSafe(64)
	if err != nil {
		return Credential{}, err
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Credential{}, fmt.Errorf("start OAuth callback listener: %w", err)
	}
	defer listener.Close()
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/auth/callback", listener.Addr().(*net.TCPAddr).Port)
	resultChannel := make(chan callbackResult, 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		result := callbackResult{
			Code: query.Get("code"), State: query.Get("state"), ClientID: query.Get("client_id"),
			Scope: query.Get("scope"), Error: query.Get("error"), Message: query.Get("error_description"),
		}
		select {
		case resultChannel <- result:
		default:
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(writer, "<!doctype html><title>Codex Bridge</title><h1>Authorization received</h1><p>%s</p><p>You may close this window.</p>", html.EscapeString(callbackMessage(result)))
	})
	server.Handler = mux
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	parameters := url.Values{
		"client_id":             {clientID},
		"ext_agent_host_id":     {hostID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"},
		"resource":              {resource},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
	}
	if returning {
		if existing.IDToken != "" {
			parameters.Set("id_token_hint", existing.IDToken)
		}
		if existing.Email != "" {
			parameters.Set("login_hint", existing.Email)
		}
	} else {
		parameters.Set("agent_name_hint", options.AppName)
	}
	authorizationURL := authorizeEndpoint + "?" + parameters.Encode()
	if options.OpenBrowser {
		if err := openURL(authorizationURL); err != nil {
			options.PrintURL(authorizationURL)
		}
	} else {
		options.PrintURL(authorizationURL)
	}

	waitCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	var callback callbackResult
	select {
	case <-waitCtx.Done():
		return Credential{}, fmt.Errorf("wait for OAuth callback: %w", waitCtx.Err())
	case callback = <-resultChannel:
	}
	if callback.State != state {
		return Credential{}, fmt.Errorf("OAuth callback state did not match")
	}
	if callback.Error != "" {
		return Credential{}, fmt.Errorf("OAuth authorization failed: %s: %s", callback.Error, callback.Message)
	}
	if callback.Code == "" {
		return Credential{}, fmt.Errorf("OAuth callback did not contain an authorization code")
	}
	if returning {
		if callback.ClientID != "" && callback.ClientID != existing.ClientID {
			return Credential{}, fmt.Errorf("OAuth callback returned a different client ID")
		}
		clientID = existing.ClientID
	} else {
		if callback.ClientID == "" || callback.ClientID == "dynamic_agent_client" {
			return Credential{}, fmt.Errorf("OAuth registration did not return an issued client ID")
		}
		clientID = callback.ClientID
	}

	token, err := exchangeCode(waitCtx, options.HTTPClient, clientID, callback.Code, verifier, redirectURI)
	if err != nil {
		return Credential{}, err
	}
	if token.IDToken == "" {
		return Credential{}, fmt.Errorf("OAuth token response did not contain an ID token")
	}
	claims, err := validateIDToken(waitCtx, options.HTTPClient, token.IDToken, clientID, nonce)
	if err != nil {
		return Credential{}, err
	}
	scope := token.Scope
	if scope == "" {
		scope = callback.Scope
	}
	credential := Credential{
		Email: claims.Email, Issuer: claims.Issuer, Subject: claims.Subject, ClientID: clientID,
		AgentHostID: hostID, IDToken: token.IDToken, AccessToken: token.AccessToken,
		RefreshToken: token.RefreshToken, TokenType: token.TokenType, ExpiresIn: token.ExpiresIn,
		Scopes: strings.Fields(scope), SavedAt: time.Now().UTC(),
	}
	if returning && credential.Subject != existing.Subject {
		return Credential{}, fmt.Errorf("reauthorized account does not match the selected saved account")
	}
	if !credential.HasScope("chatgpt.tokens.use.direct") {
		return Credential{}, fmt.Errorf("ChatGPT plan usage was not granted")
	}
	if err := SaveCredential(options.CredentialPath, credential); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func exchangeCode(ctx context.Context, client HTTPDoer, clientID, code, verifier, redirectURI string) (tokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
		"resource":      {resource},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return tokenResponse{}, fmt.Errorf("exchange authorization code: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var token tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return tokenResponse{}, fmt.Errorf("decode OAuth token response: %w", err)
	}
	if token.AccessToken == "" {
		return tokenResponse{}, fmt.Errorf("OAuth token response did not contain an access token")
	}
	return token, nil
}

func Logout(ctx context.Context, path string, client HTTPDoer) error {
	credential, err := LoadCredential(path)
	if errors.Is(err, ErrNotLoggedIn) {
		return nil
	}
	if err != nil {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	var remoteErr error
	if credential.RefreshToken != "" {
		configuration, configErr := loadOIDCConfiguration(ctx, client)
		if configErr != nil {
			remoteErr = configErr
		} else if configuration.RevocationEndpoint != "" {
			form := url.Values{"token": {credential.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {credential.ClientID}}
			request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, configuration.RevocationEndpoint, strings.NewReader(form.Encode()))
			if requestErr != nil {
				remoteErr = requestErr
			} else {
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				response, requestErr := client.Do(request)
				if requestErr != nil {
					remoteErr = requestErr
				} else {
					_ = response.Body.Close()
					if response.StatusCode != http.StatusOK {
						remoteErr = fmt.Errorf("token revocation returned HTTP %d", response.StatusCode)
					}
				}
			}
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove local credentials: %w", err)
	}
	if remoteErr != nil {
		return fmt.Errorf("local credentials removed, but remote revocation was not confirmed: %w", remoteErr)
	}
	return nil
}

func loadOrCreateHostID(credentialPath string) (string, error) {
	path := filepath.Join(filepath.Dir(credentialPath), "host.json")
	data, err := os.ReadFile(path)
	if err == nil {
		var record struct {
			HostID string `json:"ext_agent_host_id"`
		}
		if json.Unmarshal(data, &record) == nil && strings.HasPrefix(record.HostID, "urn:uuid:") {
			return record.HostID, nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read host ID: %w", err)
	}
	random, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	random[6] = (random[6] & 0x0f) | 0x40
	random[8] = (random[8] & 0x3f) | 0x80
	hostID := fmt.Sprintf("urn:uuid:%08x-%04x-%04x-%04x-%012x", random[0:4], random[4:6], random[6:8], random[8:10], random[10:16])
	record, _ := json.MarshalIndent(map[string]string{"ext_agent_host_id": hostID}, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create host ID directory: %w", err)
	}
	if err := os.WriteFile(path, append(record, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("save host ID: %w", err)
	}
	return hostID, nil
}

func randomURLSafe(length int) (string, error) {
	data, err := randomBytes(length)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func randomBytes(length int) ([]byte, error) {
	data := make([]byte, length)
	if _, err := rand.Read(data); err != nil {
		return nil, fmt.Errorf("generate cryptographic random value: %w", err)
	}
	return data, nil
}

func callbackMessage(result callbackResult) string {
	if result.Error != "" {
		return "Authorization failed: " + result.Error
	}
	return "Authorization completed successfully."
}

func openURL(value string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", value)
	case "darwin":
		command = exec.Command("open", value)
	default:
		command = exec.Command("xdg-open", value)
	}
	return command.Start()
}
