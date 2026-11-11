package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSignIn(t *testing.T) {
	email := "ola@example.org"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		out := map[string]any{}
		switch r.URL.Path {
		case "/v24.0/oauth/access_token":
			if r.Form.Get("code") != "c" || r.Form.Get("code_verifier") != "ver" || r.Form.Get("client_secret") != "sec" ||
				r.Form.Get("redirect_uri") != "https://cms/auth/callback" {
				t.Errorf("token form %v", r.Form)
			}
			out["access_token"] = "short"
		case "/v24.0/me":
			if r.Form.Get("appsecret_proof") != (&Client{AppSecret: "sec"}).proof("short") || r.Form.Get("fields") != "id,name,email" {
				t.Errorf("me form %v", r.Form)
			}
			out["id"], out["name"] = "fb1", "Ola"
			if email != "" {
				out["email"] = email
			}
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := New("app", "sec", "")
	c.GraphURL = srv.URL
	s := SignIn{c}

	u, _ := s.AuthCodeURL(context.Background(), "st", "nonce", "chal", "https://cms/auth/callback")
	pu, _ := url.Parse(u)
	q := pu.Query()
	if !strings.HasPrefix(u, "https://www.facebook.com/v24.0/dialog/oauth?") || q.Get("state") != "st" ||
		q.Get("code_challenge") != "chal" || q.Get("code_challenge_method") != "S256" || q.Get("scope") != "email,public_profile" ||
		q.Get("client_id") != "app" || q.Get("redirect_uri") != "https://cms/auth/callback" {
		t.Fatalf("login url %s", u)
	}

	id, err := s.Exchange(context.Background(), "c", "ver", "https://cms/auth/callback", "nonce")
	if err != nil {
		t.Fatal(err)
	}
	if id.Issuer != Issuer || id.Subject != "fb1" || id.Email != email || !id.EmailVerified || id.Name != "Ola" || !id.NoSuperAdmin {
		t.Fatalf("%+v", id)
	}

	email = ""
	id, err = s.Exchange(context.Background(), "c", "ver", "https://cms/auth/callback", "nonce")
	if err != nil || id.EmailVerified || id.Email != "" {
		t.Fatalf("no email must not be verified: %+v %v", id, err)
	}
}
