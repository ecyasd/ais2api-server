package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/product"
)

type nativeImportAdmin struct {
	AdminService
	scans int
	input string
}

func (admin *nativeImportAdmin) CookieBrowsers(context.Context) ([]CookieBrowser, error) {
	admin.scans++
	return []CookieBrowser{{Address: "127.0.0.1:9222", Email: "fixture@example.com", AuthUser: 2}}, nil
}

func (admin *nativeImportAdmin) DiscoverCookieAccounts(_ context.Context, input CookieImportInput) ([]CookieBrowser, error) {
	admin.input = input.Cookies
	return []CookieBrowser{{Email: "first@example.com"}, {Email: "second@example.com", AuthUser: 1}}, nil
}

func TestServerLeavesNativeBrowserAndCookieDiscoveryAvailable(t *testing.T) {
	admin := &nativeImportAdmin{}
	profile := product.Server()
	handler := NewHandler(nil, Config{Admin: admin, Product: &profile, AdminAuthEnabled: true, AdminUsername: "admin", AdminPassword: "synthetic-admin"})
	get := httptest.NewRequest("GET", "/api/accounts/import/browsers", nil)
	get.Header.Set("Authorization", "Bearer synthetic-admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, get)
	if w.Code != 200 || admin.scans != 1 || !strings.Contains(w.Body.String(), `"auth_user":2`) {
		t.Fatal("server profile intercepted native automatic browser discovery", w.Code)
	}
	// Even a full native state is forwarded to upstream recognition; no server
	// shortcut trusts a supplied identity instead of using native discovery.
	cookies := `{"cookies":[],"origins":[],"aistudio2api":{"source":{"email":"claimed@example.com"}}}`
	body, _ := json.Marshal(CookieImportInput{Cookies: cookies})
	post := httptest.NewRequest("POST", "/api/accounts/import/cookies/discover", strings.NewReader(string(body)))
	post.Header.Set("Authorization", "Bearer synthetic-admin")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, post)
	if w.Code != 200 || admin.input != cookies || !strings.Contains(w.Body.String(), "second@example.com") {
		t.Fatal("native Cookie recognition was bypassed or transformed", w.Code)
	}
	if strings.Contains(w.Body.String(), "claimed@example.com") {
		t.Fatal("custom offline identity shortcut remains")
	}
}
