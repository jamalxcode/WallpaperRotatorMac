package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewName(t *testing.T) {
	used := map[string]bool{}
	cases := []struct {
		s    Shown
		want string
	}{
		{Shown{Title: "Magic Stone", Path: "/x/da-1.JPG"}, "Magic Stone.jpg"},
		{Shown{Title: "Magic Stone", Path: "/x/da-2.jpg"}, "Magic Stone (2).jpg"},
		{Shown{Title: "a/b: c", Path: "/x/n.png"}, "a-b- c.png"},
		{Shown{Title: "  ", Path: "/x/n"}, "Wallpaper.jpg"},
		{Shown{Title: "..hidden", Path: "/x/n.jpg"}, "hidden.jpg"},
	}
	for _, c := range cases {
		if got := previewName(c.s, used); got != c.want {
			t.Errorf("previewName(%q) = %q, want %q", c.s.Title, got, c.want)
		}
	}
}

func TestHistoryMoveToEnd(t *testing.T) {
	h := &History{path: filepath.Join(t.TempDir(), "h.json")}
	for _, id := range []string{"a", "b", "c"} {
		h.Push(Shown{ID: id})
	}
	h.MoveToEnd("a")
	got := ""
	for _, s := range h.All() {
		got += s.ID
	}
	if got != "bca" {
		t.Errorf("order %q, want bca", got)
	}
	h.MoveToEnd("missing")
	if h.Len() != 3 {
		t.Errorf("len %d, want 3", h.Len())
	}
}

func TestLinkOrCopy(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(src, []byte("pic"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := linkOrCopy(src, dst); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(src) // the cache pruning the original must not affect the copy
	if b, err := os.ReadFile(dst); err != nil || string(b) != "pic" {
		t.Errorf("copy = %q, %v", b, err)
	}
}
