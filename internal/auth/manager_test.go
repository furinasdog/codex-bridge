package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) Do(request *http.Request) (*http.Response, error) { return fn(request) }

func TestManagerRefreshesAndPersistsCredential(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/credentials.json"
	original := Credential{
		Email: "user@example.com", ClientID: "client", AccessToken: "old", RefreshToken: "refresh",
		ExpiresIn: 1, SavedAt: time.Now().Add(-time.Hour), Scopes: []string{"chatgpt.tokens.use.direct"},
	}
	if err := SaveCredential(path, original); err != nil {
		t.Fatal(err)
	}
	client := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != tokenEndpoint {
			t.Fatalf("unexpected URL: %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"new","refresh_token":"next","expires_in":3600}`))}, nil
	})
	manager := NewManager(path, "", client)
	token, err := manager.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "new" {
		t.Fatalf("token = %q, want new", token)
	}
	saved, err := LoadCredential(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RefreshToken != "next" || !saved.HasScope("chatgpt.tokens.use.direct") {
		t.Fatalf("unexpected saved credential: %#v", saved)
	}
}

func TestManagerUsesStaticToken(t *testing.T) {
	t.Parallel()
	manager := NewManager("missing", "static", nil)
	token, err := manager.Token(context.Background())
	if err != nil || token != "static" {
		t.Fatalf("Token() = %q, %v", token, err)
	}
}
