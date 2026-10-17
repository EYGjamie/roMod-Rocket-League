package ballchasing_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing"
	"github.com/EYGjamie/auto-replay-upload/internal/ballchasing/ballchasingtest"
)

func TestBallchasingUpload(t *testing.T) {
	bc := ballchasingtest.New(t).API()
	ctx := context.Background()
	dir := t.TempDir()

	tests := []struct {
		file          string
		wantID        string
		wantDuplicate bool
		wantErr       error
	}{
		{file: "new.replay", wantID: "abc"},
		{file: "dup.replay", wantID: "old", wantDuplicate: true},
		{file: "bad.replay", wantErr: ballchasing.ErrRejected},
		{file: "busy.replay", wantErr: errors.New("any")},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(dir, tc.file)
			if err := os.WriteFile(path, []byte("replay-bytes"), 0o644); err != nil {
				t.Fatal(err)
			}
			id, dup, err := bc.Upload(ctx, path)
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("Upload() error = %v", err)
			case tc.wantErr != nil && err == nil:
				t.Fatal("Upload() error = nil, want error")
			case errors.Is(tc.wantErr, ballchasing.ErrRejected) && !errors.Is(err, ballchasing.ErrRejected):
				t.Fatalf("Upload() error = %v, want ErrRejected", err)
			case !errors.Is(tc.wantErr, ballchasing.ErrRejected) && errors.Is(err, ballchasing.ErrRejected):
				t.Fatalf("Upload() error = %v, must stay retryable", err)
			}
			if id != tc.wantID || dup != tc.wantDuplicate {
				t.Errorf("Upload() = %q, %v, want %q, %v", id, dup, tc.wantID, tc.wantDuplicate)
			}
		})
	}
}

func TestBallchasingPing(t *testing.T) {
	bc := ballchasingtest.New(t).API()
	ctx := context.Background()

	if name, err := bc.Ping(ctx); err != nil || name != "Jamie" {
		t.Errorf("Ping() = %q, %v, want Jamie, nil", name, err)
	}

	bc.Token = "falsch"
	if _, err := bc.Ping(ctx); !errors.Is(err, ballchasing.ErrBadToken) {
		t.Errorf("Ping() error = %v, want ErrBadToken", err)
	}
}

func TestBallchasingListReplays(t *testing.T) {
	bc := ballchasingtest.New(t).API()
	ctx := context.Background()

	page, err := bc.ListReplays(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.List) != 1 || page.Next == "" {
		t.Fatalf("first page = %+v, want one replay and a next link", page)
	}
	r := page.List[0]
	if r.Blue.Goals != 3 || r.Orange.Goals != 1 || r.Blue.Players[0].Name != "Jamie" || r.Groups[0].Name != "Liga" {
		t.Errorf("replay decoded as %+v", r)
	}

	page, err = bc.ListReplays(ctx, page.Next)
	if err != nil || len(page.List) != 1 || page.List[0].ID != "r2" || page.Next != "" {
		t.Errorf("second page = %+v, %v", page, err)
	}

	if _, err := bc.ListReplays(ctx, "https://example.com/api/replays?x=1"); err == nil {
		t.Error("ListReplays() followed a foreign next link")
	}
}

func TestBallchasingSubgroups(t *testing.T) {
	bc := ballchasingtest.New(t).API()
	ctx := context.Background()

	if groups, err := bc.ListGroups(ctx, ""); err != nil || len(groups) != 1 || groups[0].ID != "g1" {
		t.Errorf("ListGroups(\"\") = %+v, %v, want g1", groups, err)
	}
	if groups, err := bc.ListGroups(ctx, "g1"); err != nil || len(groups) != 1 || groups[0].ID != "g1-s1" {
		t.Errorf("ListGroups(g1) = %+v, %v, want subgroup g1-s1", groups, err)
	}
	if groups, err := bc.ListGroups(ctx, "g1-s1"); err != nil || len(groups) != 0 {
		t.Errorf("ListGroups(g1-s1) = %+v, %v, want none", groups, err)
	}

	if id, err := bc.CreateGroup(ctx, "Liga 2", "", true); err != nil || id != "neu-1" {
		t.Errorf("CreateGroup() top-level = %q, %v, want neu-1", id, err)
	}
	if id, err := bc.CreateGroup(ctx, "Spieltag 2", "g1", true); err != nil || id != "unter-1" {
		t.Errorf("CreateGroup() below g1 = %q, %v, want unter-1", id, err)
	}
	if _, err := bc.CreateGroup(ctx, "Spieltag 3", "weg", true); err == nil {
		t.Error("CreateGroup() below a missing parent succeeded")
	}
}
