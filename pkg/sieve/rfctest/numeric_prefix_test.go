package rfctest

import (
	"slices"
	"testing"
)

// Compare leading non-digits against the independent foxcpp implementation.
// The field is 12 in diffEmail; only the literal key changes here.
//
// checklist: SIEVE-MT-09
func TestDifferentialNumericPrefixes(t *testing.T) {
	for _, key := range []string{"12", "0012", "12suffix", "12 99", " 12", "\t12", "\u00a012", "\u200312", "+12", "-12", "", "x"} {
		script := `require ["fileinto", "comparator-i;ascii-numeric"]; if header :is :comparator "i;ascii-numeric" "X-Spam-Score" "` + key + `" { fileinto "hit"; }`
		got, err := ours(script)
		if err != nil {
			t.Fatal(err)
		}
		want, err := reference(t, script)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("key %q: ours %v, reference %v", key, got, want)
		}
	}
}
