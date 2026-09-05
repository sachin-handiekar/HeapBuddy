package api

import (
	"testing"
	"time"
)

func TestStorePutGet(t *testing.T) {
	s := NewStore()
	id := s.Put(&Bundle{})
	if id == "" {
		t.Fatal("Put returned empty id")
	}

	b, ok := s.Get(id)
	if !ok {
		t.Fatalf("Get(%q) = not found, want found", id)
	}
	if b.Report.Summary.ID != id {
		t.Errorf("stored summary id = %q, want %q (Put should stamp it)", b.Report.Summary.ID, id)
	}

	if _, ok := s.Get("rpt_nope"); ok {
		t.Error("Get of unknown id returned found")
	}
}

func TestStoreExpiry(t *testing.T) {
	now := time.Now()
	s := NewStore(WithMaxAge(time.Minute))
	s.now = func() time.Time { return now }

	id := s.Put(&Bundle{CreatedAt: now})
	if _, ok := s.Get(id); !ok {
		t.Fatal("report should be present before expiry")
	}

	now = now.Add(2 * time.Minute)
	if _, ok := s.Get(id); ok {
		t.Error("report should be evicted after maxAge")
	}
}

func TestStoreMaxSizeEviction(t *testing.T) {
	base := time.Now()
	s := NewStore(WithMaxSize(2), WithMaxAge(0))

	// Three reports inserted at increasing times; the oldest should be dropped.
	clock := base
	s.now = func() time.Time { return clock }
	ids := make([]string, 3)
	for i := range ids {
		clock = base.Add(time.Duration(i) * time.Second)
		ids[i] = s.Put(&Bundle{})
	}

	if _, ok := s.Get(ids[0]); ok {
		t.Error("oldest report should have been evicted by maxSize")
	}
	for _, id := range ids[1:] {
		if _, ok := s.Get(id); !ok {
			t.Errorf("recent report %q should still be present", id)
		}
	}
}
