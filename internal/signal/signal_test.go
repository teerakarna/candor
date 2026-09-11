package signal

import "testing"

func TestAtLeast(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{SeverityCritical, SeverityLow, true},
		{SeverityHigh, SeverityHigh, true},
		{SeverityLow, SeverityHigh, false},
		{SeverityMedium, SeverityCritical, false},
		{"", SeverityLow, false},           // unrecognised a never clears a real threshold
		{SeverityCritical, "bogus", false}, // unrecognised threshold never gets cleared
	}
	for _, c := range cases {
		if got := AtLeast(c.a, c.b); got != c.want {
			t.Errorf("AtLeast(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
