package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/product"
)

type serverCookieAdmin struct {
	AdminService
	mu          sync.Mutex
	imports     []AccountStateInput
	discoveries int
}

func (*serverCookieAdmin) Accounts(context.Context) ([]AdminAccount, error) {
	return []AdminAccount{{ID: "first@example.com"}}, nil
}

func (admin *serverCookieAdmin) DiscoverCookieAccounts(_ context.Context, input CookieImportInput) ([]CookieBrowser, error) {
	admin.mu.Lock()
	defer admin.mu.Unlock()
	admin.discoveries++
	return []CookieBrowser{{Email: "first@example.com", AuthUser: 0, Imported: true}, {Email: "second@example.com", AuthUser: 1}}, nil
}

func (admin *serverCookieAdmin) ImportAccountState(_ context.Context, input AccountStateInput) (AdminAccount, error) {
	admin.mu.Lock()
	defer admin.mu.Unlock()
	admin.imports = append(admin.imports, input)
	return AdminAccount{ID: input.Label, Label: input.Label, ImportCompleteness: input.ImportCompleteness}, nil
}

func cookieImportFixture() string {
	return "__Secure-1PSID=synthetic; SAPISID=synthetic; __Secure-1PAPISID=synthetic; __Secure-3PAPISID=synthetic"
}

func TestServerNativeCookieDiscoveryBatchAndBrowserBoundary(t *testing.T) {
	admin := &serverCookieAdmin{}
	profile := product.Server()
	handler := NewHandler(nil, Config{Admin: admin, Product: &profile, AdminAuthEnabled: true, AdminUsername: "admin", AdminPassword: "synthetic-admin"})
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		r.Header.Set("Authorization", "Bearer synthetic-admin")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	first := request("POST", "/api/accounts/import/cookies/discover", CookieImportInput{Cookies: cookieImportFixture()})
	if first.Code != 200 || !strings.Contains(first.Body.String(), "second@example.com") {
		t.Fatal("native account discovery unavailable", first.Code)
	}
	state, err := aistudio.ParseCookieText(cookieImportFixture())
	if err != nil {
		t.Fatal(err)
	}
	state.SetAuthExtension(aistudio.AuthExtension{
		Source: aistudio.AuthSource{Browser: "fixture", Email: "first@example.com"},
		OAuth:  &aistudio.ChromeOAuthMaterial{GaiaID: "synthetic-gaia", RefreshToken: "synthetic-source-metadata"},
	})
	wrapped, _ := json.Marshal(AccountStateInput{StorageState: state, AuthUser: 2, Locale: "en-GB", Timezone: "Europe/London"})
	full := request("POST", "/api/accounts/import/cookies/discover", CookieImportInput{Cookies: string(wrapped)})
	if full.Code != 200 || !strings.Contains(full.Body.String(), `"auth_user":2`) || !strings.Contains(full.Body.String(), `"imported":true`) {
		t.Fatal("full-state identity not recognized", full.Code, full.Body.String())
	}
	admin.mu.Lock()
	if admin.discoveries != 1 {
		t.Fatal("full-state identity must not make online discovery")
	}
	admin.mu.Unlock()
	blocked := request("POST", "/api/accounts/import/cookies", cookieImportRequest{Items: []cookieImportItem{{CDP: "127.0.0.1:9222"}}})
	if blocked.Code != 400 || !strings.Contains(blocked.Body.String(), "browser_import_disabled") {
		t.Fatal("cloud browser import was not rejected")
	}
	batch := request("POST", "/api/accounts/import/cookies", cookieImportRequest{
		Locale: "zh-CN", Timezone: "Asia/Hong_Kong", Items: []cookieImportItem{
			{Email: "first@example.com", Cookies: string(wrapped), AuthUser: 2},
			{Email: "bad@example.com", Cookies: "synthetic-private-invalid-cookie"},
			{Email: "second@example.com", Cookies: cookieImportFixture(), AuthUser: 1},
		},
	})
	if batch.Code != http.StatusAccepted {
		t.Fatal("native batch task unavailable", batch.Code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var tasks struct {
			Tasks []AccountTask `json:"tasks"`
		}
		result := request("GET", "/api/accounts/tasks", nil)
		if json.Unmarshal(result.Body.Bytes(), &tasks) == nil && len(tasks.Tasks) == 1 && tasks.Tasks[0].State != "running" {
			if tasks.Tasks[0].Items[0].State != "done" || tasks.Tasks[0].Items[1].State != "failed" || tasks.Tasks[0].Items[2].State != "done" {
				t.Fatal("one bad source prevented valid batch items")
			}
			if strings.Contains(result.Body.String(), "synthetic-private-invalid-cookie") {
				t.Fatal("invalid credential data leaked in task error")
			}
			admin.mu.Lock()
			defer admin.mu.Unlock()
			if len(admin.imports) != 2 {
				t.Fatal("batch did not save both selected accounts")
			}
			full := admin.imports[0]
			if full.Label != "first@example.com" || full.AuthUser != 2 || full.Locale != "en-GB" || full.Timezone != "Europe/London" || full.ImportCompleteness != "full_state" {
				t.Fatal("full native import metadata lost")
			}
			extension, exists, extensionErr := full.StorageState.AuthExtension()
			if extensionErr != nil || !exists || extension.OAuth == nil || extension.OAuth.RefreshToken != "synthetic-source-metadata" {
				t.Fatal("native source extension lost")
			}
			if admin.imports[1].ImportCompleteness != "partial" || admin.imports[1].AuthUser != 1 {
				t.Fatal("raw cookie import completeness/session mismatch")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("batch task did not finish")
}
