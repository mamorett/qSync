package purge

import (
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"simple.jpg", "simple.jpg"},
		{"2024/05/a.jpg", "2024/05/a.jpg"},
		{"with space.jpg", "'with space.jpg'"},
		{"it's.jpg", `'it'\''s.jpg'`},
		{"", "''"},
		{"a$b.jpg", "'a$b.jpg'"},
	}
	for _, c := range cases {
		if got := ShellQuote(c.in); got != c.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfirmPhrase(t *testing.T) {
	if got := ConfirmPhrase(3); got != "delete 3 files" {
		t.Errorf("ConfirmPhrase(3) = %q", got)
	}
}

func TestRemoteDeleteCommands_Chunking(t *testing.T) {
	var paths []string
	for i := 0; i < 1200; i++ {
		paths = append(paths, "f")
	}
	cmds, err := RemoteDeleteCommands("ssh", "host", "/root", paths, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(cmds))
	}
	if cmds[0][0] != "host" {
		t.Errorf("first arg should be host, got %q", cmds[0][0])
	}
	if !strings.HasPrefix(cmds[0][1], "rm -f --") {
		t.Errorf("command should start with rm -f --, got %q", cmds[0][1])
	}
}

func TestRemoteDeleteCommands_RejectsNewline(t *testing.T) {
	_, err := RemoteDeleteCommands("ssh", "host", "/root", []string{"bad\nname"}, 500)
	if err == nil {
		t.Fatal("expected error for path with newline")
	}
}

func TestPendingSaveLoad(t *testing.T) {
	dir := t.TempDir()
	ps := &PendingSet{Host: "h", Path: "/p", Deletions: []PendingDeletion{{Path: "b", Size: 2}, {Path: "a", Size: 1}}}
	if err := ps.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPending(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count() != 2 {
		t.Fatalf("count = %d", got.Count())
	}
	// Sorted on save.
	if got.Deletions[0].Path != "a" {
		t.Errorf("not sorted: %+v", got.Deletions)
	}
}

func TestLoadPending_AbsentIsEmpty(t *testing.T) {
	got, err := LoadPending(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Count() != 0 {
		t.Fatal("absent pending file should be empty, not error")
	}
}
