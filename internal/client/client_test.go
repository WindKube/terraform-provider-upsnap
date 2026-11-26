package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func makeToken(t *testing.T, claims tokenClaims) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc(payload) + ".sig"
}

func superuserToken(t *testing.T, exp time.Time) string {
	return makeToken(t, tokenClaims{ID: "su1", CollectionID: superusersCollectionID, Refreshable: true, Exp: exp.Unix()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type fakeServer struct {
	*httptest.Server
	logins    atomic.Int32
	refreshes atomic.Int32
	token     func() string
	mux       *http.ServeMux
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{mux: http.NewServeMux()}
	f.token = func() string { return superuserToken(t, time.Now().Add(time.Hour)) }
	f.mux.HandleFunc("POST /api/collections/_superusers/auth-with-password", func(w http.ResponseWriter, r *http.Request) {
		f.logins.Add(1)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["identity"] != "admin@example.com" || body["password"] != "secret-password" {
			writeJSON(w, 400, map[string]any{"status": 400, "message": "Failed to authenticate.", "data": map[string]any{}})
			return
		}
		writeJSON(w, 200, map[string]any{"token": f.token()})
	})
	f.mux.HandleFunc("POST /api/collections/{collection}/auth-refresh", func(w http.ResponseWriter, r *http.Request) {
		f.refreshes.Add(1)
		if r.Header.Get("Authorization") == "" {
			writeJSON(w, 401, map[string]any{"status": 401, "message": "missing token"})
			return
		}
		writeJSON(w, 200, map[string]any{"token": f.token()})
	})
	f.Server = httptest.NewServer(f.mux)
	t.Cleanup(f.Close)
	return f
}

func newTestClient(t *testing.T, f *fakeServer) *Client {
	t.Helper()
	c, err := New(Config{Endpoint: f.URL, Username: "admin@example.com", Password: "secret-password"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewValidatesConfig(t *testing.T) {
	cases := map[string]Config{
		"no endpoint":     {Username: "u", Password: "p"},
		"bad scheme":      {Endpoint: "ftp://x", Username: "u", Password: "p"},
		"no credentials":  {Endpoint: "http://x"},
		"only username":   {Endpoint: "http://x", Username: "u"},
		"malformed token": {Endpoint: "http://x", Token: "abc"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(cfg); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestTokenIsCachedAcrossRequests(t *testing.T) {
	f := newFakeServer(t)
	f.mux.HandleFunc("GET /api/collections/device_groups/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			writeJSON(w, 401, map[string]any{"message": "no auth"})
			return
		}
		writeJSON(w, 200, Group{ID: r.PathValue("id"), Name: "lab"})
	})
	c := newTestClient(t, f)
	for range 3 {
		g, err := c.Groups.Get(context.Background(), "abc")
		if err != nil {
			t.Fatal(err)
		}
		if g.Name != "lab" {
			t.Fatalf("got %q", g.Name)
		}
	}
	if n := f.logins.Load(); n != 1 {
		t.Fatalf("expected 1 login, got %d", n)
	}
}

func TestTokenIsRefreshedBeforeExpiry(t *testing.T) {
	f := newFakeServer(t)
	f.mux.HandleFunc("GET /api/collections/device_groups/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, Group{ID: "x"})
	})
	c := newTestClient(t, f)
	f.token = func() string { return superuserToken(t, time.Now().Add(time.Minute)) }
	if _, err := c.Groups.Get(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	f.token = func() string { return superuserToken(t, time.Now().Add(time.Hour)) }
	if _, err := c.Groups.Get(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if f.refreshes.Load() != 1 || f.logins.Load() != 1 {
		t.Fatalf("logins=%d refreshes=%d", f.logins.Load(), f.refreshes.Load())
	}
}

func TestUnauthorizedTriggersSingleRelogin(t *testing.T) {
	f := newFakeServer(t)
	var calls atomic.Int32
	f.mux.HandleFunc("GET /api/collections/device_groups/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			writeJSON(w, 401, map[string]any{"message": "token revoked"})
			return
		}
		writeJSON(w, 200, Group{ID: "x"})
	})
	c := newTestClient(t, f)
	if _, err := c.Groups.Get(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if f.logins.Load() != 2 {
		t.Fatalf("expected re-login, got %d logins", f.logins.Load())
	}
}

func TestBadCredentialsReturnAuthError(t *testing.T) {
	f := newFakeServer(t)
	c, err := New(Config{Endpoint: f.URL, Username: "admin@example.com", Password: "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	err = c.Authenticate(context.Background())
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %v", err)
	}
}

func TestExpiredStaticTokenIsRejected(t *testing.T) {
	f := newFakeServer(t)
	token := makeToken(t, tokenClaims{ID: "su1", CollectionID: superusersCollectionID, Refreshable: false, Exp: time.Now().Add(-time.Minute).Unix()})
	c, err := New(Config{Endpoint: f.URL, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	var authErr *AuthError
	if err := c.Authenticate(context.Background()); !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %v", err)
	}
}

func TestStaticTokenIsVerifiedAgainstOwnRecord(t *testing.T) {
	f := newFakeServer(t)
	token := makeToken(t, tokenClaims{ID: "su1", CollectionID: superusersCollectionID, Exp: time.Now().Add(time.Hour).Unix()})
	f.mux.HandleFunc("GET /api/collections/pbc_3142635823/records/su1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != token {
			writeJSON(w, 404, map[string]any{"message": "not found"})
			return
		}
		writeJSON(w, 200, map[string]any{"id": "su1"})
	})
	c, err := New(Config{Endpoint: f.URL, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ok, _ := c.IsSuperuser(context.Background()); !ok {
		t.Fatal("expected superuser")
	}

	bad := makeToken(t, tokenClaims{ID: "other", CollectionID: superusersCollectionID, Exp: time.Now().Add(time.Hour).Unix()})
	c, _ = New(Config{Endpoint: f.URL, Token: bad})
	var authErr *AuthError
	if err := c.Authenticate(context.Background()); !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %v", err)
	}
}

func TestListFollowsPagination(t *testing.T) {
	f := newFakeServer(t)
	const total = 5
	f.mux.HandleFunc("GET /api/collections/devices/records", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
		if got := r.URL.Query().Get("filter"); got != `name = "a\"b"` {
			t.Errorf("unexpected filter %q", got)
		}
		var items []Device
		for i := (page - 1) * perPage; i < min(page*perPage, total); i++ {
			items = append(items, Device{ID: fmt.Sprintf("d%d", i)})
		}
		writeJSON(w, 200, map[string]any{"page": page, "perPage": perPage, "totalItems": total, "totalPages": (total + perPage - 1) / perPage, "items": items})
	})
	c := newTestClient(t, f)
	devices, err := c.Devices.List(context.Background(), ListOptions{Filter: Eq("name", `a"b`), PerPage: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != total || devices[4].ID != "d4" {
		t.Fatalf("got %+v", devices)
	}
}

func TestFindOne(t *testing.T) {
	f := newFakeServer(t)
	f.mux.HandleFunc("GET /api/collections/users/records", func(w http.ResponseWriter, r *http.Request) {
		var items []User
		switch r.URL.Query().Get("filter") {
		case `username = "one"`:
			items = []User{{ID: "u1"}}
		case `username = "two"`:
			items = []User{{ID: "u1"}, {ID: "u2"}}
		}
		writeJSON(w, 200, map[string]any{"page": 1, "totalPages": 1, "items": items})
	})
	c := newTestClient(t, f)
	ctx := context.Background()
	if u, err := c.Users.FindOne(ctx, Eq("username", "one")); err != nil || u.ID != "u1" {
		t.Fatalf("one: %v %v", u, err)
	}
	if _, err := c.Users.FindOne(ctx, Eq("username", "two")); !errors.Is(err, ErrNotUnique) {
		t.Fatalf("two: %v", err)
	}
	if _, err := c.Users.FindOne(ctx, Eq("username", "none")); !IsNotFound(err) {
		t.Fatalf("none: %v", err)
	}
}

func TestAPIErrorsAreTyped(t *testing.T) {
	f := newFakeServer(t)
	f.mux.HandleFunc("POST /api/collections/devices/records", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 400, map[string]any{"status": 400, "message": "Failed to create record.", "data": map[string]any{
			"mac": map[string]string{"code": "validation_invalid_format", "message": "Invalid value format."},
			"ip":  map[string]string{"code": "validation_invalid_format", "message": "Invalid value format."},
		}})
	})
	f.mux.HandleFunc("GET /api/collections/devices/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, map[string]any{"status": 404, "message": "The requested resource wasn't found.", "data": map[string]any{}})
	})
	c := newTestClient(t, f)
	_, err := c.Devices.Create(context.Background(), DeviceInput{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 || apiErr.Fields["mac"].Code != "validation_invalid_format" {
		t.Fatalf("got %v", err)
	}
	want := "POST /api/collections/devices/records: HTTP 400: Failed to create record. (ip: Invalid value format.; mac: Invalid value format.)"
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
	if _, err := c.Devices.Get(context.Background(), "gone"); !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestTransientErrorsAreRetried(t *testing.T) {
	f := newFakeServer(t)
	var calls atomic.Int32
	f.mux.HandleFunc("DELETE /api/collections/ports/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	c := newTestClient(t, f)
	if err := c.Ports.Delete(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls.Load())
	}
}

func TestContextCancellationIsHonoured(t *testing.T) {
	f := newFakeServer(t)
	f.mux.HandleFunc("GET /api/upsnap/wake/{id}", func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	c := newTestClient(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := c.Power(ctx, ActionWake, "d1", true); err == nil {
		t.Fatal("expected error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("context deadline was not honoured")
	}
}

func TestPowerActions(t *testing.T) {
	f := newFakeServer(t)
	var lastQuery atomic.Value
	f.mux.HandleFunc("GET /api/upsnap/{action}/{id}", func(w http.ResponseWriter, r *http.Request) {
		lastQuery.Store(r.PathValue("action") + "/" + r.PathValue("id") + "?" + r.URL.RawQuery)
		if r.PathValue("action") == "wakegroup" {
			writeJSON(w, 200, []Device{{ID: "d1"}, {ID: "d2"}})
			return
		}
		writeJSON(w, 200, Device{ID: r.PathValue("id")})
	})
	c := newTestClient(t, f)
	ctx := context.Background()
	if err := c.Power(ctx, ActionShutdown, "d1", false); err != nil {
		t.Fatal(err)
	}
	if got := lastQuery.Load(); got != "shutdown/d1?async=true" {
		t.Fatalf("got %v", got)
	}
	if err := c.Power(ctx, ActionWake, "d1", true); err != nil {
		t.Fatal(err)
	}
	if got := lastQuery.Load(); got != "wake/d1?" {
		t.Fatalf("got %v", got)
	}
	devices, err := c.GroupPower(ctx, GroupActionWake, "g1")
	if err != nil || len(devices) != 2 {
		t.Fatalf("got %v %v", devices, err)
	}
}

func TestValidateCron(t *testing.T) {
	f := newFakeServer(t)
	f.mux.HandleFunc("POST /api/upsnap/validate-cron", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["cron"] == "bad" {
			writeJSON(w, 400, map[string]any{"message": "failed to parse cron expression"})
			return
		}
		writeJSON(w, 200, "valid")
	})
	c := newTestClient(t, f)
	if err := c.ValidateCron(context.Background(), "0 8 * * 1-5"); err != nil {
		t.Fatal(err)
	}
	if err := c.ValidateCron(context.Background(), "bad"); err == nil {
		t.Fatal("expected error")
	}
	if f.logins.Load() != 0 {
		t.Fatal("validate-cron must not require auth")
	}
}
