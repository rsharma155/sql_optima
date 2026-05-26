package handlers

import (
	"net/http"
	"net/url"
	"testing"
)

func TestParseSQLServerDatabaseScope(t *testing.T) {
	t.Run("omitted auto pick", func(t *testing.T) {
		db, auto := ParseSQLServerDatabaseScope(httptestReq(t, nil))
		if db != "" || !auto {
			t.Fatalf("got db=%q auto=%v", db, auto)
		}
	})
	t.Run("all databases", func(t *testing.T) {
		db, auto := ParseSQLServerDatabaseScope(httptestReq(t, map[string]string{"database": "all"}))
		if db != "" || auto {
			t.Fatalf("got db=%q auto=%v", db, auto)
		}
	})
	t.Run("empty means all", func(t *testing.T) {
		db, auto := ParseSQLServerDatabaseScope(httptestReq(t, map[string]string{"database": ""}))
		if db != "" || auto {
			t.Fatalf("got db=%q auto=%v", db, auto)
		}
	})
	t.Run("specific database", func(t *testing.T) {
		db, auto := ParseSQLServerDatabaseScope(httptestReq(t, map[string]string{"database": "AdventureWorks"}))
		if db != "AdventureWorks" || auto {
			t.Fatalf("got db=%q auto=%v", db, auto)
		}
	})
}

func httptestReq(t *testing.T, q map[string]string) *http.Request {
	t.Helper()
	u := &url.URL{Path: "/"}
	if len(q) > 0 {
		vals := url.Values{}
		for k, v := range q {
			vals.Set(k, v)
		}
		u.RawQuery = vals.Encode()
	}
	return &http.Request{URL: u}
}
