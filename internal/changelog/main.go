// Command changelog builds release notes from the fragments in changelog.d.
//
// A pull request that changes behaviour adds one fragment file and does not
// edit CHANGELOG.md, so two pull requests never conflict there. A fragment is
// named <area>-<slug>.md (sieve-variables.md) and holds one line:
//
//	<Heading>: <release note text>
//
// where Heading is one of the CHANGELOG.md headings: Added, Changed,
// Changed (breaking), Fixed or Known gaps. README.md in the directory explains
// the convention and is not a fragment.
//
// Usage:
//
//	go run ./internal/changelog [-dir changelog.d]
//	go run ./internal/changelog [-dir changelog.d] [-file CHANGELOG.md] --release <version> <date>
//
// Without --release the command prints the notes, with the fragments sorted by
// file name and grouped under their headings. With --release it inserts a
// "## [version] - date" section below "## [Unreleased]" in the changelog,
// writes the notes into it and deletes the fragment files it consumed. It
// refuses to run when the version already has a section, when a fragment is
// malformed, or when there is no fragment.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

var (
	namePattern    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+\.md$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
	headings       = []string{"Changed (breaking)", "Added", "Changed", "Fixed", "Known gaps"}
)

const (
	readme     = "README.md"
	unreleased = "## [Unreleased]"
)

// fragment is one file of the fragment directory.
type fragment struct {
	name    string
	heading string
	text    string
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("changelog", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "changelog.d", "the directory of fragments")
	file := fs.String("file", "CHANGELOG.md", "the changelog to rewrite with --release")
	release := fs.Bool("release", false, "write the fragments into a new release section; takes <version> <date>")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*release && fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "changelog: unexpected arguments; did you mean --release <version> <date>?")
		return 2
	}
	if *release && fs.NArg() != 2 {
		_, _ = fmt.Fprintln(stderr, "usage: changelog --release <version> <date>")
		return 2
	}

	frags, err := readFragments(*dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "changelog:", err)
		return 1
	}
	if !*release {
		_, _ = fmt.Fprint(stdout, render(frags))
		return 0
	}

	version, date := fs.Arg(0), fs.Arg(1)
	if err := releaseFile(*file, *dir, version, date, frags); err != nil {
		_, _ = fmt.Fprintln(stderr, "changelog:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "%s: added %s with %d fragment(s); removed them from %s\n", *file, version, len(frags), *dir)
	return 0
}

// readFragments reads and checks every fragment of dir, sorted by file name.
func readFragments(dir string) ([]fragment, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var frags []fragment
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == readme || strings.HasPrefix(name, ".") {
			continue
		}
		if !namePattern.MatchString(name) {
			return nil, fmt.Errorf("%s: name must be <area>-<slug>.md in lower case, for example sieve-variables.md", filepath.Join(dir, name))
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		f, err := parseFragment(name, string(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(dir, name), err)
		}
		frags = append(frags, f)
	}
	sort.Slice(frags, func(i, j int) bool { return frags[i].name < frags[j].name })
	return frags, nil
}

// parseFragment checks that data is one "<Heading>: <text>" line.
func parseFragment(name, data string) (fragment, error) {
	line := strings.TrimSpace(data)
	if strings.Contains(line, "\n") {
		return fragment{}, errors.New("want one line, found several")
	}
	for _, h := range headings {
		// The line is trimmed, so text after "Heading: " is never empty.
		if text, ok := strings.CutPrefix(line, h+": "); ok {
			return fragment{name: name, heading: h, text: strings.TrimSpace(text)}, nil
		}
	}
	return fragment{}, fmt.Errorf("line must start with one of %s, then a colon and a space", strings.Join(headings, ", "))
}

// render groups the fragments under their headings, in the order of headings.
// Within a heading the fragments keep their order, so a sorted input prints
// sorted by file name. The result ends in one newline, or is empty.
func render(frags []fragment) string {
	var b strings.Builder
	for _, h := range headings {
		first := true
		for _, f := range frags {
			if f.heading != h {
				continue
			}
			if first {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString("### " + h + "\n\n")
				first = false
			}
			b.WriteString("- " + f.text + "\n")
		}
	}
	return b.String()
}

// releaseFile writes the fragments into a new release section of file and
// removes them from dir.
func releaseFile(file, dir, version, date string, frags []fragment) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("version %q must look like 1.2.3", version)
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return fmt.Errorf("date %q must look like 2006-01-02", date)
	}
	if len(frags) == 0 {
		return fmt.Errorf("no fragments in %s; nothing to release", dir)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	out, err := insertRelease(string(data), version, date, render(frags))
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if err := os.WriteFile(file, []byte(out), 0o644); err != nil {
		return err
	}
	for _, f := range frags {
		if err := os.Remove(filepath.Join(dir, f.name)); err != nil {
			return fmt.Errorf("%s was updated but a fragment is left: %w", file, err)
		}
	}
	return nil
}

// insertRelease puts a release section holding notes at the end of the
// Unreleased section of the changelog text. Text already in the Unreleased
// section stays where it is.
func insertRelease(changelog, version, date, notes string) (string, error) {
	lines := strings.Split(changelog, "\n")
	versionHeading := regexp.MustCompile(`^## \[?v?` + regexp.QuoteMeta(version) + `\]?( |$)`)
	start := -1
	for i, l := range lines {
		switch {
		case versionHeading.MatchString(l):
			return "", fmt.Errorf("version %s already has a section (line %d)", version, i+1)
		case strings.TrimRight(l, " \t\r") == unreleased:
			start = i
		}
	}
	if start < 0 {
		return "", fmt.Errorf("no %q heading", unreleased)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}

	section := []string{"## [" + version + "] - " + date, ""}
	section = append(section, strings.Split(notes, "\n")...) // notes end in a newline, so this ends in ""
	var out []string
	out = append(out, lines[:end]...)
	if end > 0 && strings.TrimSpace(lines[end-1]) != "" {
		out = append(out, "")
	}
	out = append(out, section...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
}
