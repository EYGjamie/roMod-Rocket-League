package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceExe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/new" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("new"))
	}))
	defer srv.Close()
	exe := filepath.Join(t.TempDir(), "tool.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }

	if err := replaceExe(context.Background(), srv.Client(), srv.URL+"/gone", exe); err == nil {
		t.Fatal("404 accepted")
	}
	if got := read(exe); got != "old" {
		t.Fatalf("failed download changed the program: %q", got)
	}
	if err := replaceExe(context.Background(), srv.Client(), srv.URL+"/new", exe); err != nil {
		t.Fatal(err)
	}
	if got, old := read(exe), read(exe+".old"); got != "new" || old != "old" {
		t.Fatalf("exe %q, exe.old %q", got, old)
	}
}
