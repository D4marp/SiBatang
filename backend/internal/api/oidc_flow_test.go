package api

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/D4marp/SiBatang/backend/internal/config"
)

type fakeIDP struct {
	srv   *httptest.Server
	key   *rsa.PrivateKey
	nonce string
	sub   string
	amr   []string
}

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func newFakeIDP(t *testing.T) *fakeIDP {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := &fakeIDP{key: k, amr: []string{"pwd", "mfa"}}
	mux := http.NewServeMux()
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": p.srv.URL, "authorization_endpoint": p.srv.URL + "/auth", "token_endpoint": p.srv.URL + "/token", "jwks_uri": p.srv.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "k", "n": b64u(k.N.Bytes()), "e": b64u(big.NewInt(int64(k.E)).Bytes())}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "good" || r.Form.Get("code_verifier") == "" {
			http.Error(w, "bad", 400)
			return
		}
		h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k"})
		c, _ := json.Marshal(map[string]any{"iss": p.srv.URL, "sub": p.sub, "aud": "sib-web", "exp": time.Now().Add(time.Hour).Unix(), "nonce": p.nonce, "amr": p.amr})
		in := b64u(h) + "." + b64u(c)
		d := sha256.Sum256([]byte(in))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, d[:])
		json.NewEncoder(w).Encode(map[string]string{"id_token": in + "." + b64u(sig)})
	})
	return p
}

func TestOIDCLoginFlow(t *testing.T) {
	idp := newFakeIDP(t)
	h := newHarness(t, func(c *config.Config) {
		c.AppEnv, c.OIDCIssuer, c.OIDCClientID, c.OIDCRedirectURL, c.OIDCRequireMFA = "development", idp.srv.URL, "sib-web", "http://localhost/api/v1/auth/staff/callback", true
	})
	polda := h.str(`SELECT id::text FROM organizations WHERE code='POLDA_JAWA_TENGAH'`)
	reg := func(sub, role string) {
		uid := h.str(`INSERT INTO users (oidc_subject) VALUES ($1) RETURNING id::text`, sub)
		h.admin.Exec(h.ctx, `INSERT INTO memberships (user_id, role_id, scope_type, scope_id) SELECT $1, id, 'POLDA_JATENG', $3 FROM roles WHERE code=$2`, uid, role, polda)
	}
	reg("sub-monitor", "POLDA_MONITOR")

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	start := func() (*http.Cookie, url.Values) {
		resp, err := client.Get(h.ts.URL + "/api/v1/auth/staff/login")
		if err != nil || resp.StatusCode != 302 {
			t.Fatalf("login start: %v %v", err, resp)
		}
		loc, _ := url.Parse(resp.Header.Get("Location"))
		q := loc.Query()
		if !strings.HasPrefix(resp.Header.Get("Location"), idp.srv.URL+"/auth") || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
			t.Fatalf("auth redirect missing PKCE: %s", loc)
		}
		var c *http.Cookie
		for _, ck := range resp.Cookies() {
			if ck.Name == "sib_oidc" {
				c = ck
				if !ck.HttpOnly {
					t.Fatal("state cookie must be HttpOnly")
				}
			}
		}
		if c == nil {
			t.Fatal("no state cookie")
		}
		return c, q
	}
	callback := func(c *http.Cookie, state, code string) *http.Response {
		req, _ := http.NewRequest("GET", h.ts.URL+"/api/v1/auth/staff/callback?code="+code+"&state="+url.QueryEscape(state), nil)
		if c != nil {
			req.AddCookie(c)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// sukses (MFA hadir) -> sesi HttpOnly + landing Polda; sesi bisa dipakai
	c, q := start()
	idp.sub, idp.nonce = "sub-monitor", q.Get("nonce")
	resp := callback(c, q.Get("state"), "good")
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "/polda/jateng/overview" {
		t.Fatalf("callback: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	var sess *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "sib_session" {
			sess = ck
		}
	}
	if sess == nil || !sess.HttpOnly || sess.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie: %+v", sess)
	}
	if code, _, _ := h.do("GET", "/polda/jateng/metrics", nil, nil, &session{cookie: sess.Value}, false); code != 200 {
		t.Fatalf("session unusable: %d", code)
	}

	// state salah / tanpa cookie
	c, q = start()
	if r := callback(c, "tampered", "good"); r.Header.Get("Location") != "/masuk?error=state" {
		t.Fatalf("bad state: %s", r.Header.Get("Location"))
	}
	if r := callback(nil, q.Get("state"), "good"); r.Header.Get("Location") != "/masuk?error=state" {
		t.Fatalf("no cookie: %s", r.Header.Get("Location"))
	}
	// code ditolak IdP
	if r := callback(c, q.Get("state"), "evil"); r.Header.Get("Location") != "/masuk?error=idp" {
		t.Fatalf("bad code: %s", r.Header.Get("Location"))
	}
	// subject tidak terdaftar -> tidak ada auto-provision
	idp.sub, idp.nonce = "stranger", q.Get("nonce")
	if r := callback(c, q.Get("state"), "good"); r.Header.Get("Location") != "/masuk?error=not_registered" {
		t.Fatalf("unregistered: %s", r.Header.Get("Location"))
	}
	if n := h.str(`SELECT count(*)::text FROM users WHERE oidc_subject='stranger'`); n != "0" {
		t.Fatal("must not auto-provision users")
	}
	// peran privilese tanpa MFA ditolak
	idp.sub, idp.amr = "sub-monitor", []string{"pwd"}
	if r := callback(c, q.Get("state"), "good"); r.Header.Get("Location") != "/masuk?error=mfa" {
		t.Fatalf("no mfa: %s", r.Header.Get("Location"))
	}
}

func TestOIDCNotConfigured(t *testing.T) {
	h := newHarness(t)
	if code, _, body := h.do("GET", "/auth/staff/login", nil, nil, nil, false); code != 501 || body["code"] != "NOT_CONFIGURED" {
		t.Fatalf("%d %v", code, body)
	}
}
