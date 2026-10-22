package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// comparatorIdentifierScript uses encoded-character so every possible name
// octet reaches comparator validation without being interpreted as source syntax.
func comparatorIdentifierScript(name, test string) string {
	var hex strings.Builder
	for i := 0; i < len(name); i++ {
		fmt.Fprintf(&hex, "%02x ", name[i])
	}
	literal := `"${hex:` + hex.String() + `}"`
	if name == "" {
		literal = `""`
	}
	return `require ["encoded-character", "envelope", "body", "comparator-i;ascii-numeric"]; if false { if ` +
		strings.ReplaceAll(test, "COMPARATOR", literal) + ` { keep; } }`
}

var comparatorIdentifierTests = []string{
	`header :is :comparator COMPARATOR "Subject" "5"`,
	`address :is :comparator COMPARATOR "From" "5"`,
	`envelope :is :comparator COMPARATOR "from" "5"`,
	`body :is :comparator COMPARATOR "5"`,
}

// checklist: SIEVE-MT-10
func TestComparatorIdentifierASCII(t *testing.T) {
	for _, name := range []string{
		"İ;octet", "İ;ascii-casemap", "i;ascİi-casemap", "i;asciİ-casemap",
		"İ;ascii-numeric", "i;ascİi-numeric", "i;asciİ-numeric", "i;ascii-numerİc",
		"ｉ;octet", "i;оctet", "i;octet\x00", "i;octet\xff", " i;octet", "i;octet ", "",
	} {
		for _, test := range comparatorIdentifierTests {
			t.Run(name+"/"+test, func(t *testing.T) {
				src := comparatorIdentifierScript(name, test)
				if _, err := Parse(src); err == nil {
					t.Error("Parse accepted an unsupported comparator identifier in an unexecuted branch")
				}
				if err := Validate(src); err == nil {
					t.Error("Validate accepted an unsupported comparator identifier")
				}
			})
		}
	}
	// Literal UTF-8 must be rejected just like its encoded-character spelling.
	if err := Validate(`if header :comparator "İ;octet" "Subject" "5" { keep; }`); err == nil {
		t.Error("accepted a literal dotted-I comparator name")
	}
}

// checklist: SIEVE-MT-10
func TestComparatorIdentifierOctets(t *testing.T) {
	for octet := 0; octet <= 255; octet++ {
		name := string([]byte{byte(octet)}) + ";octet"
		want := octet == 'i' || octet == 'I'
		for _, test := range comparatorIdentifierTests {
			src := comparatorIdentifierScript(name, test)
			_, err := Parse(src)
			if (err == nil) != want {
				t.Errorf("Parse first octet %02x, %s: err=%v, want accepted=%v", octet, test, err, want)
			}
			if err := Validate(src); (err == nil) != want {
				t.Errorf("Validate first octet %02x, %s: err=%v, want accepted=%v", octet, test, err, want)
			}
		}
	}
	for _, name := range []string{"i;octet", "I;OCTET", "i;ascii-casemap", "I;AsCiI-CaSeMaP", "i;ascii-numeric", "I;AsCiI-NuMeRiC"} {
		for _, test := range comparatorIdentifierTests {
			if err := Validate(comparatorIdentifierScript(name, test)); err != nil {
				t.Errorf("valid ASCII name %q, %s: %v", name, test, err)
			}
		}
	}
}

// FuzzComparatorIdentifier compares acceptance with an independent reference:
// a supported name contains only ASCII and matches a registered spelling.
//
// checklist: SIEVE-MT-10
func FuzzComparatorIdentifier(f *testing.F) {
	for _, seed := range []string{"", "i;octet", "I;OCTET", "i;ascii-casemap", "i;ascii-numeric", "İ;octet", "i;ascİi-casemap", "i;ascii-numerİc", "i;octet\x00", "i;octet\xff"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 512 {
			t.Skip()
		}
		want := false
		ascii := true
		for i := 0; i < len(name); i++ {
			if name[i] >= 128 {
				ascii = false
			}
		}
		if ascii {
			for _, registered := range []string{"i;octet", "i;ascii-casemap", "i;ascii-numeric"} {
				want = want || strings.EqualFold(name, registered)
			}
		}
		for _, test := range comparatorIdentifierTests {
			src := comparatorIdentifierScript(name, test)
			_, err := Parse(src)
			if (err == nil) != want {
				t.Fatalf("Parse name %q, %s: err=%v, want accepted=%v", name, test, err, want)
			}
			if err := Validate(src); (err == nil) != want {
				t.Fatalf("Validate name %q, %s: err=%v, want accepted=%v", name, test, err, want)
			}
		}
	})
}
