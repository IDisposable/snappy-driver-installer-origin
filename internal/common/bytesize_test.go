package common

import "testing"

func TestBytesToStr(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{500, "500 B"},
		{2048, "2.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.0 GB"},
		{2 * 1024 * 1024 * 1024 * 1024, "2.0 TB"},
	}
	for _, c := range cases {
		if got := BytesToStr(c.in); got != c.want {
			t.Errorf("BytesToStr(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
