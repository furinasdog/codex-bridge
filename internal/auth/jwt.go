package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

type idClaims struct {
	Issuer   string          `json:"iss"`
	Subject  string          `json:"sub"`
	Audience json.RawMessage `json:"aud"`
	Email    string          `json:"email"`
	Nonce    string          `json:"nonce"`
	Expires  int64           `json:"exp"`
}

type oidcConfiguration struct {
	Issuer             string `json:"issuer"`
	JWKSURI            string `json:"jwks_uri"`
	RevocationEndpoint string `json:"revocation_endpoint"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KeyID     string `json:"kid"`
	KeyType   string `json:"kty"`
	Algorithm string `json:"alg"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

func validateIDToken(ctx context.Context, client HTTPDoer, rawToken, clientID, nonce string) (idClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return idClaims{}, fmt.Errorf("ID token is not a JWT")
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return idClaims{}, fmt.Errorf("decode ID token header: %w", err)
	}
	if header.Algorithm != "RS256" || header.KeyID == "" {
		return idClaims{}, fmt.Errorf("unsupported ID token signature algorithm %q", header.Algorithm)
	}

	configuration, err := loadOIDCConfiguration(ctx, client)
	if err != nil {
		return idClaims{}, err
	}
	key, err := loadRSAKey(ctx, client, configuration.JWKSURI, header.KeyID)
	if err != nil {
		return idClaims{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return idClaims{}, fmt.Errorf("decode ID token signature: %w", err)
	}
	hash := crypto.SHA256.New()
	_, _ = hash.Write([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash.Sum(nil), signature); err != nil {
		return idClaims{}, fmt.Errorf("verify ID token signature: %w", err)
	}

	var claims idClaims
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return idClaims{}, fmt.Errorf("decode ID token claims: %w", err)
	}
	if claims.Issuer != configuration.Issuer || claims.Subject == "" {
		return idClaims{}, fmt.Errorf("ID token has invalid issuer or subject")
	}
	if !audienceContains(claims.Audience, clientID) {
		return idClaims{}, fmt.Errorf("ID token audience does not contain the issued client ID")
	}
	if claims.Nonce != nonce {
		return idClaims{}, fmt.Errorf("ID token nonce does not match the authorization request")
	}
	if time.Now().Unix() >= claims.Expires {
		return idClaims{}, fmt.Errorf("ID token has expired")
	}
	return claims, nil
}

func loadOIDCConfiguration(ctx context.Context, client HTTPDoer) (oidcConfiguration, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, revocationConfig, nil)
	if err != nil {
		return oidcConfiguration{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return oidcConfiguration{}, fmt.Errorf("load OpenAI OIDC configuration: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return oidcConfiguration{}, fmt.Errorf("load OpenAI OIDC configuration: HTTP %d", response.StatusCode)
	}
	var configuration oidcConfiguration
	if err := json.NewDecoder(response.Body).Decode(&configuration); err != nil {
		return oidcConfiguration{}, fmt.Errorf("decode OpenAI OIDC configuration: %w", err)
	}
	if configuration.Issuer == "" || configuration.JWKSURI == "" {
		return oidcConfiguration{}, fmt.Errorf("OpenAI OIDC configuration is incomplete")
	}
	return configuration, nil
}

func loadRSAKey(ctx context.Context, client HTTPDoer, uri, keyID string) (*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("load OpenAI signing keys: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8<<10))
		return nil, fmt.Errorf("load OpenAI signing keys: HTTP %d", response.StatusCode)
	}
	var set jwks
	if err := json.NewDecoder(response.Body).Decode(&set); err != nil {
		return nil, fmt.Errorf("decode OpenAI signing keys: %w", err)
	}
	for _, candidate := range set.Keys {
		if candidate.KeyID != keyID || candidate.KeyType != "RSA" {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(candidate.Modulus)
		if err != nil {
			return nil, fmt.Errorf("decode signing key modulus: %w", err)
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(candidate.Exponent)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			return nil, fmt.Errorf("decode signing key exponent")
		}
		padded := make([]byte, 4)
		copy(padded[4-len(exponentBytes):], exponentBytes)
		return &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: int(binary.BigEndian.Uint32(padded))}, nil
	}
	return nil, fmt.Errorf("OpenAI signing key %q was not found", keyID)
}

func decodeJWTPart(part string, target any) error {
	data, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func audienceContains(raw json.RawMessage, expected string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == expected
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, value := range multiple {
		if value == expected {
			return true
		}
	}
	return false
}
