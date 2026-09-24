package gatesentryDnsServer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
)

func writeLegacyBlockList(t *testing.T, base, body string) string {
	t.Helper()
	path := filepath.Join(base, legacyBlockListFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMigrateLegacyBlockListAddsEntriesToEveryPolicy(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()
	if err := svc.CreateGroup(gatesentryPolicy.PolicyGroup{ID: "kids", Name: "Kids", BlockedDomains: []string{"tiktok.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir() + string(os.PathSeparator)
	path := writeLegacyBlockList(t, base, `{"keywords":[{"Content":"snapads.com","Score":0},{"Content":"https://Bad.example/path","Score":0},{"Content":"tiktok.com","Score":0}]}`)

	if err := migrateLegacyBlockList(svc, base); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	if got, want := snap.Groups[gatesentryPolicy.DefaultGroupID].BlockedDomains, []string{"snapads.com", "bad.example", "tiktok.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default blocked domains = %v, want %v", got, want)
	}
	if got, want := snap.Groups["kids"].BlockedDomains, []string{"tiktok.com", "snapads.com", "bad.example"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kids blocked domains = %v, want %v", got, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"keywords":[]}` {
		t.Fatalf("legacy block list after migration = %s, want it emptied", raw)
	}

	// A second start finds the list empty and changes nothing.
	if err := migrateLegacyBlockList(svc, base); err != nil {
		t.Fatal(err)
	}
	if got := svc.Snapshot().Groups["kids"].BlockedDomains; len(got) != 3 {
		t.Fatalf("second migration changed kids blocked domains: %v", got)
	}
}

func TestMigrateLegacyBlockListSkipsTheUneditedStockList(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()
	base := t.TempDir() + string(os.PathSeparator)
	path := writeLegacyBlockList(t, base, `{"keywords":[{"Content":"snapads.com","Score":0}]}`)

	if err := migrateLegacyBlockList(svc, base); err != nil {
		t.Fatal(err)
	}
	if got := svc.Snapshot().Groups[gatesentryPolicy.DefaultGroupID].BlockedDomains; len(got) != 0 {
		t.Fatalf("stock list was copied into the default policy: %v", got)
	}
	if raw, _ := os.ReadFile(path); string(raw) != `{"keywords":[]}` {
		t.Fatalf("stock list was not retired: %s", raw)
	}
}

func TestMigrateLegacyBlockListWithoutFileIsANoop(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()
	if err := migrateLegacyBlockList(svc, t.TempDir()+string(os.PathSeparator)); err != nil {
		t.Fatal(err)
	}
}
