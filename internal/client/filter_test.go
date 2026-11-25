package client

import "testing"

func TestQuote(t *testing.T) {
	cases := map[string]string{
		`plain`:          `"plain"`,
		`it's`:           `"it's"`,
		`say "hi"`:       `"say \"hi\""`,
		`back\slash`:     `"back\\slash"`,
		`" || id != "`:   `"\" || id != \""`,
		`trailing\`:      `"trailing\\"`,
		`unicode ✓ name`: `"unicode ✓ name"`,
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestAnd(t *testing.T) {
	if got := And(Eq("a", "1"), "", AnyEq("groups", "g")); got != `(a = "1") && (groups ?= "g")` {
		t.Fatalf("got %s", got)
	}
	if got := And(); got != "" {
		t.Fatalf("got %s", got)
	}
}
