package mastodon

import "testing"

func TestIsBridgedID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"01KZH4YY0SXVYMT4RWZS6RXZYT", false},
		{"", false},
		{"https://mastodon.social/users/Gargron/statuses/1", true},
		{"http://instance.example/users/a/statuses/2", true},
	}
	for _, c := range cases {
		if got := IsBridgedID(c.id); got != c.want {
			t.Errorf("IsBridgedID(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestBridgedStatusID(t *testing.T) {
	cases := []struct {
		id   string
		want string
		ok   bool
	}{
		// a federated Warpnet reply carries its parent in the query
		{"https://gw.example/users/01KUSER/statuses/01KTWEET?parent=https%3A%2F%2Fm%2F1", "01KTWEET", true},
		{"https://mastodon.social/users/Gargron/statuses/117043346514398510", "117043346514398510", true},
		{"01KZH4YY0SXVYMT4RWZS6RXZYT", "", false}, // native id
		{"https://mastodon.social/users/Gargron", "", false},
		{"https://gw.example/users/u/statuses/", "", false},
		{"https://gw.example/users/u/statuses/1/activity", "", false},
	}
	for _, c := range cases {
		got, ok := BridgedStatusID(c.id)
		if got != c.want || ok != c.ok {
			t.Errorf("BridgedStatusID(%q) = (%q, %v), want (%q, %v)", c.id, got, ok, c.want, c.ok)
		}
	}
}
