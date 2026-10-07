package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/config"
)

// OAuth logins are stored per service in ~/.cav/oauth/<service>.json with owner-only
// permissions. The file holds the access and refresh tokens and the client id. The client
// secret stays in its own key source; the file only names that source.
type oauthState struct {
	ClientID     string    `json:"clientID"`
	SecretSource string    `json:"secretSource"`
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	Expires      time.Time `json:"expires"`
}

// OAuthProvider describes one service's OAuth2 endpoints.
type OAuthProvider struct {
	AuthorizeURL string
	TokenURL     string
}

var OAuthProviders = map[string]OAuthProvider{
	// https://freesound.org/docs/api/authentication.html
	"freesound": {
		AuthorizeURL: "https://freesound.org/apiv2/oauth2/authorize/",
		TokenURL:     "https://freesound.org/apiv2/oauth2/access_token/",
	},
}

func oauthPath(service string) string {
	return filepath.Join(config.Home(), "oauth", service+".json")
}

func loadOAuth(service string) (*oauthState, error) {
	b, err := os.ReadFile(oauthPath(service))
	if err != nil {
		return nil, err
	}
	s := &oauthState{}
	return s, json.Unmarshal(b, s)
}

func saveOAuth(service string, s *oauthState) error {
	dir := filepath.Dir(oauthPath(service))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := oauthPath(service) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, oauthPath(service))
}

// LoggedIn reports whether an OAuth login exists for the service.
func LoggedIn(service string) bool {
	_, err := loadOAuth(service)
	return err == nil
}

// AuthorizeURL is the page the user opens to grant access.
func AuthorizeURL(service, clientID string) (string, error) {
	p, ok := OAuthProviders[service]
	if !ok {
		return "", fmt.Errorf("%s has no OAuth login", service)
	}
	q := url.Values{"client_id": {clientID}, "response_type": {"code"}, "state": {"cav"}}
	return p.AuthorizeURL + "?" + q.Encode(), nil
}

// Login exchanges an authorization code for tokens and stores them.
func Login(ctx context.Context, service, clientID, secretSource, code string) error {
	p, ok := OAuthProviders[service]
	if !ok {
		return fmt.Errorf("%s has no OAuth login", service)
	}
	secret, err := Resolve(ctx, service, secretSource)
	if err != nil {
		return err
	}
	st := &oauthState{ClientID: clientID, SecretSource: secretSource}
	if err := tokenRequest(ctx, p.TokenURL, url.Values{
		"client_id": {clientID}, "client_secret": {secret},
		"grant_type": {"authorization_code"}, "code": {strings.TrimSpace(code)},
	}, st); err != nil {
		return err
	}
	return saveOAuth(service, st)
}

func oauthToken(ctx context.Context, service string) (string, error) {
	st, err := loadOAuth(service)
	if err != nil {
		return "", &KeyError{Msg: "not logged in to " + service, Missing: true, Fix: "cav config login " + service}
	}
	if time.Until(st.Expires) > time.Minute {
		return st.AccessToken, nil
	}
	p := OAuthProviders[service]
	secret, err := Resolve(ctx, service, st.SecretSource)
	if err != nil {
		return "", err
	}
	if err := tokenRequest(ctx, p.TokenURL, url.Values{
		"client_id": {st.ClientID}, "client_secret": {secret},
		"grant_type": {"refresh_token"}, "refresh_token": {st.RefreshToken},
	}, st); err != nil {
		return "", &KeyError{Msg: "the " + service + " login expired and could not be refreshed: " + err.Error(), Fix: "cav config login " + service}
	}
	if err := saveOAuth(service, st); err != nil {
		return "", err
	}
	return st.AccessToken, nil
}

func tokenRequest(ctx context.Context, tokenURL string, form url.Values, st *oauthState) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != 200 || body.AccessToken == "" {
		if body.Error != "" {
			return errors.New(body.Error)
		}
		return fmt.Errorf("token request failed: HTTP %d", resp.StatusCode)
	}
	st.AccessToken, st.RefreshToken = body.AccessToken, body.RefreshToken
	st.Expires = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	return nil
}
