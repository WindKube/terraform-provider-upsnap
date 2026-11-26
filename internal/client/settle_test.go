package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type deviceFields struct {
	Description string `json:"description"`
}

// settleServer serves one device; revert decides what a GET returns after the n-th GET.
type settleServer struct {
	*fakeServer
	mu      sync.Mutex
	stored  Device
	gets    atomic.Int32
	patches atomic.Int32
}

func newSettleServer(t *testing.T, revert func(gets int32, d *Device)) *settleServer {
	s := &settleServer{fakeServer: newFakeServer(t), stored: Device{ID: "d1", Description: "old", Updated: "t0"}}
	s.mux.HandleFunc("PATCH /api/collections/devices/records/d1", func(w http.ResponseWriter, r *http.Request) {
		n := s.patches.Add(1)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.stored.Description = body["description"]
		s.stored.Updated = "patch-" + strconv.Itoa(int(n))
		writeJSON(w, 200, s.stored)
	})
	s.mux.HandleFunc("GET /api/collections/devices/records/d1", func(w http.ResponseWriter, _ *http.Request) {
		n := s.gets.Add(1)
		s.mu.Lock()
		defer s.mu.Unlock()
		revert(n, &s.stored)
		writeJSON(w, 200, s.stored)
	})
	return s
}

func settleClient(t *testing.T, s *settleServer, settle time.Duration) *Client {
	t.Helper()
	c, err := New(Config{Endpoint: s.URL, Username: "admin@example.com", Password: "secret-password", SettleTime: settle})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func settleDescription(ctx context.Context, c *Client) (*Device, error) {
	write := func(ctx context.Context) (*Device, error) {
		return c.Devices.Update(ctx, "d1", map[string]string{"description": "new"})
	}
	d, err := write(ctx)
	if err != nil {
		return nil, err
	}
	return c.Devices.Settle(ctx, "d1", d, Watch[Device]{
		Fields:  func(d *Device) any { return deviceFields{Description: d.Description} },
		Rewrite: write,
	})
}

func TestSettleRewritesAfterRevert(t *testing.T) {
	s := newSettleServer(t, func(gets int32, d *Device) {
		if gets == 2 {
			d.Description, d.Updated = "old", "ping"
		}
	})
	d, err := settleDescription(context.Background(), settleClient(t, s, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if d.Description != "new" || s.stored.Description != "new" {
		t.Fatalf("description = %q, stored %q", d.Description, s.stored.Description)
	}
	if got := s.patches.Load(); got != 2 {
		t.Fatalf("patches = %d, want 2", got)
	}
}

func TestSettleIgnoresUnwatchedChanges(t *testing.T) {
	s := newSettleServer(t, func(gets int32, d *Device) {
		d.Status, d.Updated = "online", "ping"
	})
	d, err := settleDescription(context.Background(), settleClient(t, s, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.patches.Load(); got != 1 {
		t.Fatalf("patches = %d, want 1", got)
	}
	if d.Status != "online" {
		t.Fatalf("Settle should return the latest record, got status %q", d.Status)
	}
}

func TestSettleGivesUp(t *testing.T) {
	s := newSettleServer(t, func(_ int32, d *Device) {
		d.Description, d.Updated = "old", "ping"
	})
	_, err := settleDescription(context.Background(), settleClient(t, s, time.Second))
	var reverted *RevertedError
	if !errors.As(err, &reverted) {
		t.Fatalf("err = %v, want RevertedError", err)
	}
	if !slices.Equal(reverted.Fields, []string{"description"}) || reverted.Rewrites != maxRewrites || reverted.Updated != "ping" {
		t.Fatalf("unexpected error contents: %+v", reverted)
	}
	if got := s.patches.Load(); got != maxRewrites+1 {
		t.Fatalf("patches = %d, want %d", got, maxRewrites+1)
	}
}

func TestSettleDisabled(t *testing.T) {
	s := newSettleServer(t, func(int32, *Device) {})
	if _, err := settleDescription(context.Background(), settleClient(t, s, 0)); err != nil {
		t.Fatal(err)
	}
	if got := s.gets.Load(); got != 0 {
		t.Fatalf("gets = %d, want 0", got)
	}
}

func TestSettleHonoursContext(t *testing.T) {
	s := newSettleServer(t, func(int32, *Device) {})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := settleDescription(ctx, settleClient(t, s, time.Minute)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}
