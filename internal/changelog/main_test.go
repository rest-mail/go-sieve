package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseChangelog = `# Changelog

Intro text.

## [Unreleased]

## [0.8.0] - 2026-11-05

### Added

- Older entry.
`

// write creates the files (name to content) in dir.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// read returns the content of a file.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// fixture makes a fragment directory and a changelog in a new temporary
// directory and returns the directory of fragments and the changelog path.
func fixture(t *testing.T, changelog string, fragments map[string]string) (string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "changelog.d")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, fragments)
	file := filepath.Join(root, "CHANGELOG.md")
	write(t, root, map[string]string{"CHANGELOG.md": changelog})
	return dir, file
}

// execute runs the command and returns its exit code and combined output.
func execute(args ...string) (int, string) {
	var out bytes.Buffer
	code := run(args, &out, &out)
	return code, out.String()
}

// notSuperuser skips a test that needs a permission error to occur.
func notSuperuser(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop the superuser")
	}
}

var sampleFragments = map[string]string{
	"sieve-variables.md":   "Added: The variables extension (RFC 5229).\n",
	"imap-fix-order.md":    "Fixed: Messages sort by arrival.\n",
	"sieve-comparator.md":  "Known gaps: A `:comparator` name cannot come from a variable.\n",
	"sieve-addresses.md":   "Added: Address lists.\n",
	"api-rename.md":        "Changed (breaking): `Foo` is now `Bar`.\n",
	"docs-readme.md":       "Changed: README layout.\n",
	"README.md":            "How this directory works.\n",
	".gitkeep":             "",
	"zzz-last-in-fixed.md": "Fixed: Last fix.",
}

func TestPrintSortsAndGroups(t *testing.T) {
	dir, _ := fixture(t, baseChangelog, sampleFragments)
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, out := execute("-dir", dir)
	want := `### Changed (breaking)

- ` + "`Foo` is now `Bar`." + `

### Added

- Address lists.
- The variables extension (RFC 5229).

### Changed

- README layout.

### Fixed

- Messages sort by arrival.
- Last fix.

### Known gaps

- A ` + "`:comparator`" + ` name cannot come from a variable.
`
	if code != 0 || out != want {
		t.Fatalf("exit %d, output:\n%s\nwant:\n%s", code, out, want)
	}
}

func TestPrintWithoutFragments(t *testing.T) {
	dir, _ := fixture(t, baseChangelog, map[string]string{"README.md": "x"})
	if code, out := execute("-dir", dir); code != 0 || out != "" {
		t.Fatalf("exit %d, output %q, want an empty success", code, out)
	}
}

func TestReleaseInsertsSectionAndRemovesFragments(t *testing.T) {
	dir, file := fixture(t, baseChangelog, map[string]string{
		"sieve-b.md": "Added: Second.\n",
		"sieve-a.md": "Added: First.\n",
		"sieve-c.md": "Fixed: A fix.\n",
		"README.md":  "Keep me.\n",
	})
	code, out := execute("-dir", dir, "-file", file, "--release", "0.9.0", "2026-11-20")
	if code != 0 || !strings.Contains(out, "added 0.9.0 with 3 fragment(s)") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	want := `# Changelog

Intro text.

## [Unreleased]

## [0.9.0] - 2026-11-20

### Added

- First.
- Second.

### Fixed

- A fix.

## [0.8.0] - 2026-11-05

### Added

- Older entry.
`
	if got := read(t, file); got != want {
		t.Fatalf("changelog:\n%s\nwant:\n%s", got, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "README.md" {
		t.Fatalf("directory after release: %v, %v; want only README.md", entries, err)
	}
}

func TestReleaseKeepsTextInUnreleased(t *testing.T) {
	changelog := "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- By hand.\n\n## [0.8.0] - 2026-11-05\n"
	dir, file := fixture(t, changelog, map[string]string{"sieve-a.md": "Added: New.\n"})
	if code, out := execute("-dir", dir, "-file", file, "--release", "0.9.0", "2026-11-20"); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	want := "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- By hand.\n\n" +
		"## [0.9.0] - 2026-11-20\n\n### Added\n\n- New.\n\n## [0.8.0] - 2026-11-05\n"
	if got := read(t, file); got != want {
		t.Fatalf("changelog:\n%s\nwant:\n%s", got, want)
	}
}

func TestReleaseWhenUnreleasedIsLast(t *testing.T) {
	for _, tail := range []string{"", "\n", "- By hand."} {
		dir, file := fixture(t, "# Changelog\n\n## [Unreleased]\n"+tail, map[string]string{"sieve-a.md": "Added: New.\n"})
		if code, out := execute("-dir", dir, "-file", file, "--release", "1.0.0", "2026-11-20"); code != 0 {
			t.Fatalf("tail %q: exit %d:\n%s", tail, code, out)
		}
		got := read(t, file)
		if !strings.HasSuffix(got, "\n## [1.0.0] - 2026-11-20\n\n### Added\n\n- New.\n") || strings.HasSuffix(got, "\n\n") {
			t.Fatalf("tail %q: changelog:\n%q", tail, got)
		}
	}
}

func TestReleaseRefusesExistingVersion(t *testing.T) {
	for _, version := range []string{"0.8.0", "1.2.3-rc.1"} {
		changelog := baseChangelog + "\n## v1.2.3-rc.1\n"
		dir, file := fixture(t, changelog, map[string]string{"sieve-a.md": "Added: New.\n"})
		code, out := execute("-dir", dir, "-file", file, "--release", version, "2026-11-20")
		if code != 1 || !strings.Contains(out, "version "+version+" already has a section") {
			t.Fatalf("%s: exit %d:\n%s", version, code, out)
		}
		if got := read(t, file); got != changelog {
			t.Fatalf("%s: changelog changed:\n%s", version, got)
		}
		if _, err := os.Stat(filepath.Join(dir, "sieve-a.md")); err != nil {
			t.Fatalf("%s: fragment removed: %v", version, err)
		}
	}
}

func TestReleaseVersionPrefixIsNotADuplicate(t *testing.T) {
	// 0.8 and 10.8.0 must not make 0.8.0 look taken.
	changelog := "## [Unreleased]\n\n## [10.8.0] - 2026-11-05\n\n## [0.8.01] - 2026-11-04\n"
	dir, file := fixture(t, changelog, map[string]string{"sieve-a.md": "Added: New.\n"})
	if code, out := execute("-dir", dir, "-file", file, "--release", "0.8.0", "2026-11-20"); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestReleaseProblems(t *testing.T) {
	good := map[string]string{"sieve-a.md": "Added: New.\n"}
	tests := []struct {
		name      string
		changelog string
		fragments map[string]string
		args      []string
		code      int
		want      string
	}{
		{"bad version", baseChangelog, good, []string{"--release", "v1", "2026-11-20"}, 1, `version "v1" must look like 1.2.3`},
		{"bad date", baseChangelog, good, []string{"--release", "1.0.0", "20 Nov"}, 1, `date "20 Nov" must look like 2006-01-02`},
		{"impossible date", baseChangelog, good, []string{"--release", "1.0.0", "2026-02-30"}, 1, `date "2026-02-30" must look like`},
		{"no fragments", baseChangelog, map[string]string{"README.md": "x"}, []string{"--release", "1.0.0", "2026-11-20"}, 1, "nothing to release"},
		{"no unreleased heading", "# Changelog\n\n## [0.8.0] - 2026-11-05\n", good, []string{"--release", "1.0.0", "2026-11-20"}, 1, `no "## [Unreleased]" heading`},
		{"missing changelog", "", good, []string{"-file", "no-such-file", "--release", "1.0.0", "2026-11-20"}, 1, "no-such-file"},
		{"one argument", baseChangelog, good, []string{"--release", "1.0.0"}, 2, "usage: changelog --release <version> <date>"},
		{"arguments without release", baseChangelog, good, []string{"1.0.0", "2026-11-20"}, 2, "did you mean --release"},
		{"unknown flag", baseChangelog, good, []string{"--nope"}, 2, "flag provided but not defined"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, file := fixture(t, tc.changelog, tc.fragments)
			// A later -file in tc.args wins over this one.
			args := append([]string{"-dir", dir, "-file", file}, tc.args...)
			code, out := execute(args...)
			if code != tc.code || !strings.Contains(out, tc.want) {
				t.Fatalf("exit %d, want %d and %q:\n%s", code, tc.code, tc.want, out)
			}
			if tc.changelog != "" && read(t, file) != tc.changelog {
				t.Fatalf("changelog changed:\n%s", read(t, file))
			}
		})
	}
}

func TestFragmentProblems(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"Upper-Case.md", "Added: x\n", "name must be <area>-<slug>.md"},
		{"noslug.md", "Added: x\n", "name must be <area>-<slug>.md"},
		{"notes-text.txt", "Added: x\n", "name must be <area>-<slug>.md"},
		{"area-two-lines.md", "Added: x\nAdded: y\n", "want one line, found several"},
		{"area-no-heading.md", "Just text.\n", "line must start with one of Changed (breaking), Added, Changed, Fixed, Known gaps"},
		{"area-wrong-case.md", "added: x\n", "line must start with one of"},
		{"area-no-space.md", "Added:x\n", "line must start with one of"},
		{"area-empty.md", "", "line must start with one of"},
		{"area-no-text.md", "Added:  \n", "line must start with one of"}, // trimmed to "Added:"
		{"area-blank-text.md", "Fixed: \t.\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := fixture(t, baseChangelog, map[string]string{tc.name: tc.content})
			code, out := execute("-dir", dir)
			if tc.want == "" {
				if code != 0 {
					t.Fatalf("exit %d:\n%s", code, out)
				}
				return
			}
			if code != 1 || !strings.Contains(out, tc.want) || !strings.Contains(out, tc.name) {
				t.Fatalf("exit %d, want 1 and %q naming %s:\n%s", code, tc.want, tc.name, out)
			}
		})
	}
}

func TestMissingFragmentDirectory(t *testing.T) {
	code, out := execute("-dir", filepath.Join(t.TempDir(), "absent"))
	if code != 1 || !strings.Contains(out, "absent") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestUnreadableFragment(t *testing.T) {
	notSuperuser(t)
	dir, _ := fixture(t, baseChangelog, map[string]string{"sieve-a.md": "Added: x\n"})
	if err := os.Chmod(filepath.Join(dir, "sieve-a.md"), 0); err != nil {
		t.Fatal(err)
	}
	if code, out := execute("-dir", dir); code != 1 || !strings.Contains(out, "permission denied") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestChangelogNotWritable(t *testing.T) {
	notSuperuser(t)
	dir, file := fixture(t, baseChangelog, map[string]string{"sieve-a.md": "Added: x\n"})
	if err := os.Chmod(file, 0o444); err != nil {
		t.Fatal(err)
	}
	code, out := execute("-dir", dir, "-file", file, "--release", "1.0.0", "2026-11-20")
	if code != 1 || !strings.Contains(out, "permission denied") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "sieve-a.md")); err != nil {
		t.Fatalf("fragment removed after a failed write: %v", err)
	}
}

func TestFragmentNotRemovable(t *testing.T) {
	notSuperuser(t)
	dir, file := fixture(t, baseChangelog, map[string]string{"sieve-a.md": "Added: x\n"})
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	code, out := execute("-dir", dir, "-file", file, "--release", "1.0.0", "2026-11-20")
	if code != 1 || !strings.Contains(out, "was updated but a fragment is left") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}
