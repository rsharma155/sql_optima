package setup

import (
	"strings"
	"testing"
)

func TestResolveMigrationsDir(t *testing.T) {
	dir, err := ResolveMigrationsDir()
	if err != nil {
		t.Skipf("migration dir not in this checkout layout: %v", err)
	}
	if !strings.HasSuffix(dir, "sql_scripts") {
		t.Fatalf("unexpected dir: %s", dir)
	}
}
