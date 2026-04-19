package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	p := New(Config{ClientID: "cid-123"})
	u := p.AuthorizeURL("state-x", "https://app.example.com/cb")
	if !strings.HasPrefix(u, "https://github.com/login/oauth/authorize?") {
		t.Errorf("url: %s", u)
	}
	for _, want := range []string{"client_id=cid-123", "state=state-x", "scope=read", "redirect_uri=https%3A%2F%2Fapp.example.com%2Fcb"} {
		if !strings.Contains(u, want) {
			t.Errorf("url 缺 %q: %s", want, u)
		}
	}
}

func TestExchange_Success(t *testing.T) {
	var tokReq, userReq int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			tokReq++
			_ = r.ParseForm()
			if r.FormValue("code") != "the-code" {
				t.Errorf("code: %q", r.FormValue("code"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"gh-token"}`))
		case r.URL.Path == "/user":
			userReq++
			if r.Header.Get("Authorization") != "Bearer gh-token" {
				t.Errorf("auth: %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":99,"login":"alice","name":"Alice","email":"alice@x.com"}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p := New(Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		TokenURL:     srv.URL + "/token",
		APIBase:      srv.URL,
	})
	prof, err := p.Exchange(context.Background(), "the-code", "cb")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if prof.ExternalID != "99" || prof.Email != "alice@x.com" || prof.Name != "Alice" {
		t.Errorf("profile: %+v", prof)
	}
	if tokReq != 1 || userReq != 1 {
		t.Errorf("reqs: tok=%d user=%d", tokReq, userReq)
	}
}

func TestExchange_PrimaryEmailFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"t"}`))
		case "/user":
			// 主资料里 email 为 null
			_, _ = w.Write([]byte(`{"id":1,"login":"bob","email":null}`))
		case "/user/emails":
			_, _ = w.Write([]byte(`[
				{"email":"secondary@x","primary":false,"verified":true},
				{"email":"primary@x","primary":true,"verified":true}
			]`))
		default:
			t.Errorf("path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p := New(Config{
		ClientID: "c", ClientSecret: "s",
		TokenURL: srv.URL + "/token",
		APIBase:  srv.URL,
	})
	prof, err := p.Exchange(context.Background(), "code", "cb")
	if err != nil {
		t.Fatal(err)
	}
	if prof.Email != "primary@x" {
		t.Errorf("primary email: %q", prof.Email)
	}
}

func TestExchange_TokenEndpointFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad_verification_code"})
	}))
	defer srv.Close()
	p := New(Config{ClientID: "c", ClientSecret: "s", TokenURL: srv.URL, APIBase: srv.URL})
	_, err := p.Exchange(context.Background(), "bad", "cb")
	if err == nil {
		t.Error("400 应 error")
	}
}

func TestExchange_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer srv.Close()
	p := New(Config{ClientID: "c", ClientSecret: "s", TokenURL: srv.URL, APIBase: srv.URL})
	_, err := p.Exchange(context.Background(), "x", "cb")
	if err == nil {
		t.Error("空 token 应 error")
	}
}

func TestName(t *testing.T) {
	if New(Config{}).Name() != "github" {
		t.Error("name")
	}
}
