package oidc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type idp struct {
	key   *rsa.PrivateKey
	srv   *httptest.Server
	nonce string
	sub   string
	amr   []string
	aud   string
	exp   time.Time
	alg   string
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func newIDP(t *testing.T) *idp {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := &idp{key: k, sub: "user-1", amr: []string{"pwd", "mfa"}, aud: "cid", exp: time.Now().Add(time.Hour), alg: "RS256"}
	mux := http.NewServeMux()
	p.srv = httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": p.srv.URL, "authorization_endpoint": p.srv.URL + "/auth",
			"token_endpoint": p.srv.URL + "/token", "jwks_uri": p.srv.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "k1",
			"n": b64(k.N.Bytes()), "e": b64(big.NewInt(int64(k.E)).Bytes())}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") == "" {
			http.Error(w, "bad", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id_token": p.token()})
	})
	t.Cleanup(p.srv.Close)
	return p
}

func (p *idp) token() string {
	h, _ := json.Marshal(map[string]string{"alg": p.alg, "kid": "k1"})
	c, _ := json.Marshal(map[string]any{"iss": p.srv.URL, "sub": p.sub, "aud": p.aud, "exp": p.exp.Unix(), "nonce": p.nonce, "amr": p.amr, "name": "Tester"})
	in := b64(h) + "." + b64(c)
	d := sha256.Sum256([]byte(in))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, d[:])
	return in + "." + b64(sig)
}

func client(p *idp) *Client {
	return New(Config{Issuer: p.srv.URL, ClientID: "cid", RedirectURL: "http://app/cb"})
}

func TestExchangeAndVerify(t *testing.T) {
	p := newIDP(t)
	p.nonce = "n1"
	cl, err := client(p).Exchange(context.Background(), "good-code", "verifier", "n1")
	if err != nil || cl.Subject != "user-1" || !cl.HasAMR("mfa") {
		t.Fatalf("%v %+v", err, cl)
	}
	u, err := client(p).AuthURL(context.Background(), "st", "n1", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if want := "code_challenge=" + PKCEChallenge("verifier"); !contains([]string{u}, u) || len(u) == 0 || !(len(u) > 0 && indexOf(u, want) >= 0) {
		t.Fatalf("auth url lacks PKCE challenge: %s", u)
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestRejectsBadTokens(t *testing.T) {
	cases := map[string]func(p *idp){
		"nonce":    func(p *idp) { p.nonce = "other" },
		"audience": func(p *idp) { p.aud = "someone-else" },
		"expired":  func(p *idp) { p.exp = time.Now().Add(-time.Hour) },
		"alg":      func(p *idp) { p.alg = "none" },
	}
	for name, mutate := range cases {
		p := newIDP(t)
		p.nonce = "n1"
		mutate(p)
		if _, err := client(p).Exchange(context.Background(), "good-code", "v", "n1"); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
	p := newIDP(t)
	p.nonce = "n1"
	if _, err := client(p).Exchange(context.Background(), "wrong-code", "v", "n1"); err == nil {
		t.Error("bad code must fail")
	}
	// tanda tangan dirusak
	tok := p.token()
	if _, err := client(p).Verify(context.Background(), tok[:len(tok)-4]+"AAAA", "n1"); err == nil {
		t.Error("tampered signature must fail")
	}
}
