package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	p := New(Config{ClientID: "gc"})
	u := p.AuthorizeURL("st", "https://x/cb")
	if !strings.Contains(u, "accounts.google.com") {
		t.Errorf("url: %s", u)
	}
	for _, want := range []string{"client_id=gc", "state=st", "scope=openid+email+profile", "response_type=code"} {
		if !strings.Contains(u, want) {
			t.Errorf("缺 %q: %s", want, u)
		}
	}
}

func TestExchange_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if r.FormValue("grant_type") != "authorization_code" {
				t.Errorf("grant_type: %q", r.FormValue("grant_type"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"gat"}`))
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer gat" {
				t.Errorf("auth header: %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"sub":"sub-123","email":"user@x.com","email_verified":true,"name":"User"}`))
		}
	}))
	defer srv.Close()

	p := New(Config{
		ClientID: "c", ClientSecret: "s",
		TokenURL:    srv.URL + "/token",
		UserInfoURL: srv.URL + "/userinfo",
	})
	prof, err := p.Exchange(context.Background(), "code", "cb")
	if err != nil {
		t.Fatal(err)
	}
	if prof.ExternalID != "sub-123" || prof.Email != "user@x.com" || prof.Name != "User" {
		t.Errorf("profile: %+v", prof)
	}
}

func TestExchange_TokenFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	p := New(Config{TokenURL: srv.URL, UserInfoURL: srv.URL})
	_, err := p.Exchange(context.Background(), "x", "cb")
	if err == nil {
		t.Error("400 应 error")
	}
}

func TestExchange_UserInfoFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"x"}`))
		case "/userinfo":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
		}
	}))
	defer srv.Close()
	p := New(Config{
		TokenURL:    srv.URL + "/token",
		UserInfoURL: srv.URL + "/userinfo",
	})
	_, err := p.Exchange(context.Background(), "code", "cb")
	if err == nil {
		t.Error("userinfo 401 应 error")
	}
}

func TestName(t *testing.T) {
	if New(Config{}).Name() != "google" {
		t.Error("name")
	}
}
