package gatesentry2logger

import (
	"fmt"
	"testing"
	"time"

	"github.com/tidwall/buntdb"
)

func newActivityTestLogger(t *testing.T) *Log {
	t.Helper()
	l := NewLogger(t.TempDir() + "/activity.db")
	t.Cleanup(func() { _ = l.Database.Close() })
	return l
}

func insertActivityEntry(t *testing.T, l *Log, at int64, ip, url string) {
	t.Helper()
	value := fmt.Sprintf(`{"time": %d, "ip":%q, "url":%q, "type":"dns"}`, at, ip, url)
	err := l.Database.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(fmt.Sprintf("entry-%d", at), value, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetDeviceActivityFiltersAndOrdersNewestFirst(t *testing.T) {
	l := newActivityTestLogger(t)
	now := time.Now().Unix()
	insertActivityEntry(t, l, now-30, "192.0.2.10", "older-ipv4.example")
	insertActivityEntry(t, l, now-20, "2001:db8::1", "ipv6.example")
	insertActivityEntry(t, l, now-10, "192.0.2.10", "newer-ipv4.example")
	// Outside the query window: must never appear.
	insertActivityEntry(t, l, now-5000, "192.0.2.10", "expired.example")
	// Other clients: must never appear.
	insertActivityEntry(t, l, now-5, "198.51.100.7", "other-client.example")

	entries, err := l.GetDeviceActivity([]string{"192.0.2.10", "2001:db8::1"}, 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3: %+v", len(entries), entries)
	}
	if entries[0].URL != "newer-ipv4.example" ||
		entries[1].URL != "ipv6.example" ||
		entries[2].URL != "older-ipv4.example" {
		t.Fatalf("order = [%s %s %s], want newest first", entries[0].URL, entries[1].URL, entries[2].URL)
	}
}

func TestGetDeviceActivityRespectsLimit(t *testing.T) {
	l := newActivityTestLogger(t)
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		insertActivityEntry(t, l, now-int64(60-i), "192.0.2.10", fmt.Sprintf("entry-%d.example", i))
	}
	entries, err := l.GetDeviceActivity([]string{"192.0.2.10"}, 3600, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want limit of 2", len(entries))
	}
	if entries[0].URL != "entry-4.example" || entries[1].URL != "entry-3.example" {
		t.Fatalf("limited order = [%s %s], want the two newest", entries[0].URL, entries[1].URL)
	}
}

func TestGetDeviceActivityDefaultsAndEmptyMatch(t *testing.T) {
	l := newActivityTestLogger(t)
	now := time.Now().Unix()
	// Just inside the 24h default window.
	insertActivityEntry(t, l, now-3600, "192.0.2.10", "within-day.example")
	// A zero/zero call uses the documented defaults.
	entries, err := l.GetDeviceActivity([]string{"192.0.2.10"}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].URL != "within-day.example" {
		t.Fatalf("default-window entries = %+v", entries)
	}

	// No match addresses means no query rather than matching everything.
	none, err := l.GetDeviceActivity(nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("empty match set returned %d entries", len(none))
	}
}
