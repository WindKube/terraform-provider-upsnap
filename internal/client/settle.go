package client

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
)

const (
	settlePoll  = 250 * time.Millisecond
	maxRewrites = 3
)

// Watch describes a write to verify with Settle. Fields returns the fields the write set, as a
// struct with json tags; server-maintained fields such as status or updated must not be part of it.
type Watch[T any] struct {
	Fields  func(*T) any
	Rewrite func(context.Context) (*T, error)
}

// RevertedError reports that UpSnap kept overwriting a write.
type RevertedError struct {
	Collection string
	ID         string
	Fields     []string
	Updated    string
	Rewrites   int
}

func (e *RevertedError) Error() string {
	return fmt.Sprintf("UpSnap reverted %s/%s %d times after it was written (fields %s, last overwritten at %s). "+
		"UpSnap's ping job saves the device and port records it loaded before pinging, which reverts changes made while the ping runs; "+
		"a slow custom ping_cmd, many port checks or an in-flight wake/shutdown/reboot widen that window. "+
		"Raise the provider's settle_time to wait longer for these writes to settle",
		e.Collection, e.ID, e.Rewrites, strings.Join(e.Fields, ", "), e.Updated)
}

// Settle watches record id for the client's settle time after written was returned by a write.
// If the watched fields change, the write is repeated straight away, at most maxRewrites times;
// right after UpSnap's stale save is the safest moment, as its next snapshot is a full ping
// interval away.
func (s Collection[T]) Settle(ctx context.Context, id string, written *T, w Watch[T]) (*T, error) {
	if s.c.settleTime <= 0 {
		return written, nil
	}
	want := w.Fields(written)
	rewrites := 0
	deadline := time.Now().Add(s.c.settleTime)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(settlePoll):
		}
		cur, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		changed := diffFields(want, w.Fields(cur))
		if len(changed) == 0 {
			written = cur
			continue
		}
		if rewrites == maxRewrites {
			return nil, &RevertedError{Collection: s.name, ID: id, Fields: changed, Updated: updatedAt(cur), Rewrites: rewrites}
		}
		rewrites++
		if written, err = w.Rewrite(ctx); err != nil {
			return nil, err
		}
		want = w.Fields(written)
		deadline = time.Now().Add(s.c.settleTime)
	}
	return written, nil
}

func diffFields(want, got any) []string {
	wv, gv := reflect.ValueOf(want), reflect.ValueOf(got)
	if wv.Kind() != reflect.Struct || wv.Type() != gv.Type() {
		if reflect.DeepEqual(want, got) {
			return nil
		}
		return []string{"(record)"}
	}
	var changed []string
	for i := range wv.NumField() {
		if reflect.DeepEqual(wv.Field(i).Interface(), gv.Field(i).Interface()) {
			continue
		}
		name, _, _ := strings.Cut(wv.Type().Field(i).Tag.Get("json"), ",")
		if name == "" {
			name = wv.Type().Field(i).Name
		}
		changed = append(changed, name)
	}
	return changed
}

func updatedAt(v any) string {
	if u, ok := v.(interface{ UpdatedAt() string }); ok {
		return u.UpdatedAt()
	}
	return "unknown"
}
