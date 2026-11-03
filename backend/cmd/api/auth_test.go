package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthSessionTokenPrefersBearerAndFallsBackToCookie(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: "widoroku_session", Value: "cookie-token"})
	request.Header.Set("Authorization", "Bearer bearer-token")
	if token := authSessionToken(request); token != "bearer-token" {
		t.Fatalf("authSessionToken() = %q, want bearer token", token)
	}

	request = httptest.NewRequest("GET", "/api/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: "widoroku_session", Value: "cookie-token"})
	if token := authSessionToken(request); token != "cookie-token" {
		t.Fatalf("authSessionToken() = %q, want cookie token", token)
	}
}
