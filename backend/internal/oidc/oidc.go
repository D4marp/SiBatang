// Package oidc: klien OpenID Connect (Authorization Code + PKCE) hanya dengan stdlib.
// Memverifikasi id_token (RS256/ES256) terhadap JWKS issuer: iss, aud, exp, nbf, nonce. alg lain (none/HS*) ditolak.
package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Issuer, ClientID, ClientSecret, RedirectURL string
	HTTP                                        *http.Client
	Now                                         func() time.Time
}

type Client struct {
	cfg  Config
	mu   sync.Mutex
	meta *metadata
	keys map[string]crypto.PublicKey
	at   time.Time
}

type metadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type Claims struct {
	Subject string
	Name    string
	AMR     []string
}

func (c Claims) HasAMR(v string) bool {
	for _, a := range c.AMR {
		if a == v {
			return true
		}
	}
	return false
}

func New(cfg Config) *Client {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Issuer = strings.TrimRight(cfg.Issuer, "/")
	return &Client{cfg: cfg}
}

func (c *Client) getJSON(ctx context.Context, u string, v any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	res, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("oidc: GET %s: status %d", u, res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(v)
}

func (c *Client) discover(ctx context.Context) (*metadata, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.meta != nil {
		return c.meta, nil
	}
	var m metadata
	if err := c.getJSON(ctx, c.cfg.Issuer+"/.well-known/openid-configuration", &m); err != nil {
		return nil, err
	}
	if strings.TrimRight(m.Issuer, "/") != c.cfg.Issuer || m.AuthorizationEndpoint == "" || m.TokenEndpoint == "" || m.JWKSURI == "" {
		return nil, errors.New("oidc: discovery document does not match configured issuer")
	}
	c.meta = &m
	return c.meta, nil
}

// PKCEChallenge = BASE64URL(SHA256(verifier)) (S256).
func PKCEChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (c *Client) AuthURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	m, err := c.discover(ctx)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(m.AuthorizationEndpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", c.cfg.ClientID)
	q.Set("redirect_uri", c.cfg.RedirectURL)
	q.Set("scope", "openid profile")
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("code_challenge", PKCEChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Exchange menukar code (+ verifier PKCE) dan memverifikasi id_token. Mengembalikan klaim tervalidasi.
func (c *Client) Exchange(ctx context.Context, code, verifier, nonce string) (*Claims, error) {
	m, err := c.discover(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {c.cfg.RedirectURL},
		"client_id": {c.cfg.ClientID}, "code_verifier": {verifier}}
	if c.cfg.ClientSecret != "" {
		form.Set("client_secret", c.cfg.ClientSecret)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.TokenEndpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("oidc: token endpoint status %d", res.StatusCode)
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok); err != nil || tok.IDToken == "" {
		return nil, errors.New("oidc: token response has no id_token")
	}
	return c.Verify(ctx, tok.IDToken, nonce)
}

func (c *Client) key(ctx context.Context, kid string) (crypto.PublicKey, error) {
	m, err := c.discover(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok && c.cfg.Now().Sub(c.at) < 10*time.Minute {
		return k, nil
	}
	if c.keys != nil && c.cfg.Now().Sub(c.at) < 10*time.Second {
		return nil, errors.New("oidc: unknown signing key") // batasi refetch JWKS
	}
	var set struct {
		Keys []struct {
			Kty, Kid, Crv, N, E, X, Y string
		} `json:"keys"`
	}
	if err := c.getJSON(ctx, m.JWKSURI, &set); err != nil {
		return nil, err
	}
	keys := map[string]crypto.PublicKey{}
	for _, k := range set.Keys {
		switch k.Kty {
		case "RSA":
			n, e := b64Int(k.N), b64Int(k.E)
			if n != nil && e != nil && e.IsInt64() {
				keys[k.Kid] = &rsa.PublicKey{N: n, E: int(e.Int64())}
			}
		case "EC":
			if k.Crv == "P-256" {
				x, y := b64Int(k.X), b64Int(k.Y)
				if x != nil && y != nil && elliptic.P256().IsOnCurve(x, y) {
					keys[k.Kid] = &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
				}
			}
		}
	}
	c.keys, c.at = keys, c.cfg.Now()
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("oidc: unknown signing key")
}

func b64Int(s string) *big.Int {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) == 0 {
		return nil
	}
	return new(big.Int).SetBytes(b)
}

type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

const skew = time.Minute

// Verify memvalidasi tanda tangan dan klaim id_token.
func (c *Client) Verify(ctx context.Context, raw, nonce string) (*Claims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("oidc: malformed token")
	}
	var hdr struct{ Alg, Kid string }
	if err := decodePart(parts[0], &hdr); err != nil {
		return nil, err
	}
	if hdr.Alg != "RS256" && hdr.Alg != "ES256" {
		return nil, errors.New("oidc: unsupported alg")
	}
	k, err := c.key(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("oidc: bad signature encoding")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	switch pk := k.(type) {
	case *rsa.PublicKey:
		if hdr.Alg != "RS256" || rsa.VerifyPKCS1v15(pk, crypto.SHA256, digest[:], sig) != nil {
			return nil, errors.New("oidc: bad signature")
		}
	case *ecdsa.PublicKey:
		if hdr.Alg != "ES256" || len(sig) != 64 || !ecdsa.Verify(pk, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
			return nil, errors.New("oidc: bad signature")
		}
	default:
		return nil, errors.New("oidc: bad key")
	}
	var cl struct {
		Iss   string   `json:"iss"`
		Sub   string   `json:"sub"`
		Aud   audience `json:"aud"`
		Exp   int64    `json:"exp"`
		Nbf   int64    `json:"nbf"`
		Nonce string   `json:"nonce"`
		Name  string   `json:"name"`
		AMR   []string `json:"amr"`
	}
	if err := decodePart(parts[1], &cl); err != nil {
		return nil, err
	}
	now := c.cfg.Now()
	switch {
	case strings.TrimRight(cl.Iss, "/") != c.cfg.Issuer:
		return nil, errors.New("oidc: issuer mismatch")
	case !contains(cl.Aud, c.cfg.ClientID):
		return nil, errors.New("oidc: audience mismatch")
	case cl.Exp == 0 || now.After(time.Unix(cl.Exp, 0).Add(skew)):
		return nil, errors.New("oidc: token expired")
	case cl.Nbf != 0 && now.Before(time.Unix(cl.Nbf, 0).Add(-skew)):
		return nil, errors.New("oidc: token not yet valid")
	case cl.Nonce == "" || cl.Nonce != nonce:
		return nil, errors.New("oidc: nonce mismatch")
	case cl.Sub == "":
		return nil, errors.New("oidc: missing subject")
	}
	return &Claims{Subject: cl.Sub, Name: cl.Name, AMR: cl.AMR}, nil
}

func decodePart(p string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return errors.New("oidc: bad token encoding")
	}
	return json.Unmarshal(b, v)
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}
