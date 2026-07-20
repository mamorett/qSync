package output

import "testing"

func TestHumanizeIEC(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{24511234, "23.4 MiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}
	for _, c := range cases {
		if got := HumanizeIEC(c.n).String(); got != c.want {
			t.Errorf("HumanizeIEC(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
