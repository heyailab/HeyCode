package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// dummyHandler 计数被调用次数，用于断言中间件是否放行。
type dummyHandler struct{ called int }

func (d *dummyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.called++
	w.WriteHeader(http.StatusOK)
}

func newRequest(t *testing.T, method, target, authHeader string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return req
}

func TestMiddleware_Disabled_AllPass(t *testing.T) {
	m := New(nil) // 未配置 token
	if m.Enabled() {
		t.Fatal("expected disabled")
	}
	mw := m.Middleware()
	if mw != nil {
		t.Fatal("disabled manager should return nil middleware")
	}
}

func TestMiddleware_Enabled(t *testing.T) {
	m := New([]string{"secret-abc"})
	if !m.Enabled() {
		t.Fatal("expected enabled")
	}

	cases := []struct {
		name       string
		authHeader string
		wantCode   int
		wantCall   bool
	}{
		{"valid token", "Bearer secret-abc", http.StatusOK, true},
		{"valid token case-insensitive scheme", "bearer secret-abc", http.StatusOK, true},
		{"missing header", "", http.StatusUnauthorized, false},
		{"wrong token", "Bearer wrong", http.StatusUnauthorized, false},
		{"malformed header", "Token secret-abc", http.StatusUnauthorized, false},
		{"empty bearer", "Bearer ", http.StatusUnauthorized, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &dummyHandler{}
			mw := m.Middleware()(d)
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, newRequest(t, http.MethodGet, "/api/servers", c.authHeader))
			if rec.Code != c.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, c.wantCode)
			}
			if d.called > 0 != c.wantCall {
				t.Fatalf("handler called = %d, want call=%v", d.called, c.wantCall)
			}
		})
	}
}

func TestMiddleware_MultiToken(t *testing.T) {
	m := New([]string{" old-token ", "", "new-token"}) // 含空白项
	if !m.Enabled() {
		t.Fatal("expected enabled")
	}

	for _, tok := range []string{"old-token", "new-token"} {
		d := &dummyHandler{}
		m.Middleware()(d).ServeHTTP(
			httptest.NewRecorder(),
			newRequest(t, http.MethodGet, "/", "Bearer "+tok),
		)
		if d.called != 1 {
			t.Fatalf("token %q should pass", tok)
		}
	}
}

func TestVerifyQuery(t *testing.T) {
	m := New([]string{"secret"})

	cases := []struct {
		target string
		want   bool
	}{
		{"/ws/sessions/abc?token=secret", true},
		{"/ws/sessions/abc?token=wrong", false},
		{"/ws/sessions/abc", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.target, nil)
		if got := m.VerifyQuery(req); got != c.want {
			t.Errorf("VerifyQuery(%s) = %v, want %v", c.target, got, c.want)
		}
	}
}

func TestVerifyQuery_Disabled(t *testing.T) {
	m := New(nil)
	req := httptest.NewRequest(http.MethodGet, "/ws/sessions/abc", nil)
	if !m.VerifyQuery(req) {
		t.Fatal("disabled manager should pass all")
	}
}
