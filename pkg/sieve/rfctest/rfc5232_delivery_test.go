package rfctest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// These tests check the parts of RFC 5232 that need the flag state the
// evaluator holds: the internal variable, the hasflag test, and the flags each
// delivery is stored with (sieve.FlagDeliverer, Outcome.ImplicitKeepFlags).

// flagExec is a recExec that also implements sieve.FlagDeliverer, recording
// each delivery as "keep:<flags>" or "fileinto:<folder>:<flags>".
type flagExec struct {
	*recExec
	deliveries []string
}

func (e *flagExec) KeepWithFlags(flags []string) {
	e.deliveries = append(e.deliveries, "keep:"+strings.Join(flags, " "))
}

func (e *flagExec) FileIntoWithFlags(folder string, _ bool, flags []string) {
	e.deliveries = append(e.deliveries, "fileinto:"+folder+":"+strings.Join(flags, " "))
}

// runFlags parses script and runs it with a flagExec.
func runFlags(t *testing.T, script string) (*flagExec, sieve.Outcome) {
	t.Helper()
	s, err := sieve.Parse(script)
	if err != nil {
		t.Fatalf("Parse(%q): %v", script, err)
	}
	ex := &flagExec{recExec: newRecExec()}
	return ex, s.Evaluate(sieveEmail(), ex)
}

// TestFlags_InternalVariable checks that without a variable name the actions
// work on the internal variable, empty when the script starts: setflag
// replaces, addflag adds and removeflag removes, with flag names compared
// case-insensitively (RFC 5232 §3, §3.1–3.3).
//
// checklist: FLAGS-16
func TestFlags_InternalVariable(t *testing.T) {
	tests := []struct {
		name    string
		actions string
		want    []string
	}{
		{"empty at start", ``, []string{}},
		{"setflag replaces", `addflag "a"; setflag "b c";`, []string{"b", "c"}},
		{"addflag adds", `addflag "a"; addflag ["b", "A"];`, []string{"a", "b"}},
		{"removeflag removes", `setflag "a b c"; removeflag "B";`, []string{"a", "c"}},
		{"removing an unset flag", `setflag "a"; removeflag "z";`, []string{"a"}},
		{"setflag to nothing", `setflag "a"; setflag "";`, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, out := runFlags(t, `require "imap4flags"; `+tt.actions)
			if !reflect.DeepEqual(out.ImplicitKeepFlags, tt.want) {
				t.Errorf("internal variable = %q, want %q", out.ImplicitKeepFlags, tt.want)
			}
		})
	}
}

// TestFlags_Hasflag checks the hasflag test on the internal variable, with the
// examples of RFC 5232 §4 (the variable examples run on the internal variable
// set to the same value): the match type defaults to :is, the comparator to
// i;ascii-casemap, a key string holding several names is a list of keys, and
// :count counts distinct flags.
//
// checklist: FLAGS-17
func TestFlags_Hasflag(t *testing.T) {
	const myVar = `setflag "NonJunk Junk gnus-forward $Forwarded NotJunk JunkRecorded $Junk $NotJunk";`
	tests := []struct {
		name  string
		state string
		test  string
		want  bool
	}{
		{"§4 :is with a space-separated key", `setflag "A B";`, `hasflag :is "b A"`, true},
		{"§4 key list", `setflag "A B";`, `hasflag ["b","A"]`, true},
		{"§4 :contains Junk", myVar, `hasflag :contains "Junk"`, true},
		{"§4 :contains forward", myVar, `hasflag :contains "forward"`, true},
		{"§4 :contains label or forward", myVar, `hasflag :contains ["label", "forward"]`, true},
		{"§4 :contains junk or forward", myVar, `hasflag :contains ["junk", "forward"]`, true},
		{"§4 junk forward in one key", myVar, `hasflag :contains "junk forward"`, true},
		{"§4 forward junk in one key", myVar, `hasflag :contains "forward junk"`, true},
		{"§4 :contains label", myVar, `hasflag :contains "label"`, false},
		{"§4 :contains label1 or label2", myVar, `hasflag :contains ["label1", "label2"]`, false},
		{"§4 :count ge 2", `setflag "A B";`, `hasflag :count "ge" :comparator "i;ascii-numeric" "2"`, true},
		{":count eq 3", `setflag "A B";`, `hasflag :count "eq" :comparator "i;ascii-numeric" "3"`, false},
		{":count of the empty variable", ``, `hasflag :count "eq" :comparator "i;ascii-numeric" "0"`, true},
		{"empty variable has no flag", ``, `hasflag "a"`, false},
		{"default comparator ignores case", `setflag "\\Seen";`, `hasflag "\\SEEN"`, true},
		{"i;octet keeps case", `setflag "Work";`, `hasflag :comparator "i;octet" "work"`, false},
		{":is matches whole flags only", `setflag "Junk";`, `hasflag "Jun"`, false},
		{":matches", `setflag "$Junk";`, `hasflag :matches "$J*"`, true},
		{"after removeflag", `setflag "a b"; removeflag "a";`, `hasflag "a"`, false},
		{"empty key", `setflag "a";`, `hasflag ""`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := `require ["imap4flags", "relational", "comparator-i;ascii-numeric", "fileinto"]; ` +
				tt.state + ` if ` + tt.test + ` { fileinto "Hit"; }`
			ex, _ := runFlags(t, script)
			got := len(ex.deliveries) == 1 && strings.HasPrefix(ex.deliveries[0], "fileinto:Hit:")
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.test, got, tt.want)
			}
		})
	}

	for _, bad := range []string{
		`if hasflag "a" { keep; }`,
		`require "imap4flags"; if hasflag "MyVar" "Junk" { keep; }`,
		`require "imap4flags"; if hasflag "MyVar" ["Junk"] { keep; }`,
		`require "imap4flags"; if hasflag :count "eq" "1" { keep; }`,
		`require "imap4flags"; if hasflag :localpart "a" { keep; }`,
		`require "imap4flags"; if hasflag :is { keep; }`,
	} {
		if err := sieve.Validate(bad); err == nil {
			t.Errorf("%q: accepted, want an error", bad)
		}
	}
}

// TestFlags_DeliveryFlags checks that keep, fileinto and the implicit keep
// store the message with the internal variable as it was when the action ran,
// or with only the :flags list when one is given (RFC 5232 §3, §5).
//
// checklist: FLAGS-18
func TestFlags_DeliveryFlags(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		want     []string
		implicit []string // nil when the implicit keep is cancelled
	}{
		{"keep uses the internal variable", `setflag "\\Seen"; keep;`, []string{`keep:\Seen`}, nil},
		{"keep :flags replaces it", `setflag "\\Seen"; keep :flags "a";`, []string{"keep:a"}, nil},
		{"keep :flags empty stores no flags", `setflag "a"; keep :flags "";`, []string{"keep:"}, nil},
		{"fileinto takes the flags of its own time", `addflag "a"; fileinto "F"; addflag "b";`, []string{"fileinto:F:a"}, nil},
		{"fileinto :copy and the implicit keep", `addflag "a"; fileinto :copy "F"; addflag "b";`, []string{"fileinto:F:a"}, []string{"a", "b"}},
		{"implicit keep uses the internal variable", `setflag "x"; addflag "y";`, nil, []string{"x", "y"}},
		{"discard leaves a flagged keep", `keep :flags "a"; discard;`, []string{"keep:a"}, nil},
		{"§5 example", `fileinto :flags "\\Deleted" "INBOX.From Boss";`, []string{`fileinto:INBOX.From Boss:\Deleted`}, nil},
		{":flags list is cleaned up", `fileinto :flags ["\\Recent", "ok", "", "OK"] "F";`, []string{"fileinto:F:ok"}, nil},
		{":flags leaves the internal variable alone", `setflag "a"; fileinto :copy :flags "b" "F";`, []string{"fileinto:F:b"}, []string{"a"}},
		{"reject delivers nothing", `fileinto "F"; reject "no";`, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex, out := runFlags(t, `require ["imap4flags", "fileinto", "copy", "reject"]; `+tt.script)
			if !reflect.DeepEqual(ex.deliveries, tt.want) {
				t.Errorf("deliveries = %q, want %q", ex.deliveries, tt.want)
			}
			if got := ex.applied; containsPrefix(got, "keep") || containsPrefix(got, "fileinto:") {
				t.Errorf("Keep or FileInto called alongside the FlagDeliverer: %q", got)
			}
			if tt.implicit == nil {
				if out.ImplicitKeep || out.ImplicitKeepFlags != nil {
					t.Errorf("implicit keep = %v with flags %q, want cancelled", out.ImplicitKeep, out.ImplicitKeepFlags)
				}
			} else if !out.ImplicitKeep || !reflect.DeepEqual(out.ImplicitKeepFlags, tt.implicit) {
				t.Errorf("implicit keep = %v with flags %q, want flags %q", out.ImplicitKeep, out.ImplicitKeepFlags, tt.implicit)
			}
		})
	}
}

// TestFlags_MergedDeliveryLastFlagsWin checks that when repeated keep or
// fileinto deliveries to one destination are merged, the message is delivered
// once with the last flag list (RFC 5232 §3; RFC 5228 §2.10.3), and that the
// merged deliveries keep the order in which each destination was first named.
//
// checklist: FLAGS-19
func TestFlags_MergedDeliveryLastFlagsWin(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   []string
	}{
		{"keep :flags twice", `keep :flags "a"; keep :flags "b";`, []string{"keep:b"}},
		{"keep after a flag change", `setflag "a"; keep; setflag "b"; keep;`, []string{"keep:b"}},
		{"fileinto twice", `setflag "a"; fileinto "F"; setflag "b"; fileinto "F";`, []string{"fileinto:F:b"}},
		{"later :flags wins", `fileinto :flags "a" "F"; fileinto "G"; fileinto :flags "c" "F";`, []string{"fileinto:F:c", "fileinto:G:"}},
		{"keep and fileinto stay apart", `keep :flags "a"; fileinto :flags "b" "F"; keep :flags "c";`, []string{"keep:c", "fileinto:F:b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex, _ := runFlags(t, `require ["imap4flags", "fileinto"]; `+tt.script)
			if !reflect.DeepEqual(ex.deliveries, tt.want) {
				t.Errorf("deliveries = %q, want %q", ex.deliveries, tt.want)
			}
		})
	}
}

func containsPrefix(list []string, prefix string) bool {
	for _, s := range list {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
