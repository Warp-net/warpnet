package deeplink

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Link
		err  error
	}{
		{"user happy path", "warpnet://user/01HZX7K8", Link{Kind: KindUser, ID: "01HZX7K8", Raw: "warpnet://user/01HZX7K8"}, nil},
		{"user trailing slash", "warpnet://user/abc/", Link{Kind: KindUser, ID: "abc", Raw: "warpnet://user/abc/"}, nil},
		{"missing slashes (shell-stripped)", "warpnet:user/abc", Link{Kind: KindUser, ID: "abc", Raw: "warpnet:user/abc"}, nil},
		{"uppercase scheme tolerated", "WARPNET://user/abc", Link{Kind: KindUser, ID: "abc", Raw: "WARPNET://user/abc"}, nil},
		{"surrounded by whitespace", "  warpnet://user/abc  ", Link{Kind: KindUser, ID: "abc", Raw: "warpnet://user/abc"}, nil},
		{"missing id", "warpnet://user/", Link{}, ErrMissingID},
		{"unknown kind", "warpnet://tweet/123", Link{}, ErrUnsupportedKind},
		{"different scheme", "https://example.com/", Link{}, ErrNotWarpnetURL},
		{"blank", "", Link{}, ErrNotWarpnetURL},
		{"garbage", "not a url", Link{}, ErrNotWarpnetURL},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Kind != tc.want.Kind || got.ID != tc.want.ID || got.Raw != tc.want.Raw {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFromArgs(t *testing.T) {
	if _, ok := FromArgs([]string{"warpnet"}); ok {
		t.Fatal("argv with no deep link should return ok=false")
	}
	if _, ok := FromArgs([]string{"warpnet", "--flag", "x"}); ok {
		t.Fatal("non-deeplink args should return ok=false")
	}
	l, ok := FromArgs([]string{"warpnet", "warpnet://user/01HQ"})
	if !ok || l.Kind != KindUser || l.ID != "01HQ" {
		t.Fatalf("expected to parse the deep-link arg, got ok=%v link=%+v", ok, l)
	}
	// Garbage warpnet:// arg surrounded by good args should yield ok=false
	// (we don't synthesize a link out of partial data).
	if _, ok := FromArgs([]string{"warpnet", "warpnet://tweet/123"}); ok {
		t.Fatal("unsupported kind should still return ok=false")
	}
}
