package repo

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// TestState is what git-farm is willing to claim about a directory's tests.
//
// The third state is the whole point. The fence is the second most meaningful
// mark on the farm and the one claim that can be wrong in public, on somebody
// else's README. A language that puts its tests inside the file it tests has no
// test *files*, and reporting that as "untested" is a false accusation. When
// the rules do not apply, say so instead of guessing.
type TestState string

const (
	TestsUnknown TestState = "unknown" // no rule here that can be trusted
	TestsFound   TestState = "found"   // test files, or inline tests, were found
	TestsMissing TestState = "none"    // the rules apply, and found nothing
)

// A language is worth knowing about only for what it says about tests: whether
// its test files have a recognisable name, and whether tests can hide inside an
// ordinary source file.
type language struct {
	name string

	// reliable means a missing test file really does mean missing tests. False
	// for languages where the convention is unclear or unknown to this table,
	// and those directories get the unknown state rather than a broken fence.
	reliable bool

	// inline is a marker that says "there are tests in this file". Scanning
	// contents is only worth it where the marker is short and unambiguous.
	inline string
}

// Extensions this tool is prepared to make a claim about. Anything missing from
// here is not a judgement about the language — it means nobody has checked what
// its convention is, and the fence stays unknown.
var languages = map[string]language{
	".go":    {name: "Go", reliable: true},
	".js":    {name: "JavaScript", reliable: true},
	".jsx":   {name: "JavaScript", reliable: true},
	".mjs":   {name: "JavaScript", reliable: true},
	".cjs":   {name: "JavaScript", reliable: true},
	".ts":    {name: "TypeScript", reliable: true},
	".tsx":   {name: "TypeScript", reliable: true},
	".py":    {name: "Python", reliable: true},
	".rb":    {name: "Ruby", reliable: true},
	".java":  {name: "Java", reliable: true},
	".kt":    {name: "Kotlin", reliable: true},
	".kts":   {name: "Kotlin", reliable: true},
	".cs":    {name: "C#", reliable: true},
	".swift": {name: "Swift", reliable: true},
	".php":   {name: "PHP", reliable: true},
	".scala": {name: "Scala", reliable: true},
	".ex":    {name: "Elixir", reliable: true},
	".exs":   {name: "Elixir", reliable: true},
	".dart":  {name: "Dart", reliable: true},

	// Tests live inside the source file. Reliable only because the contents are
	// read; without the scan these would have to be unknown.
	".rs":  {name: "Rust", reliable: true, inline: "#[cfg(test)]"},
	".zig": {name: "Zig", reliable: true, inline: "\ntest "},

	// Known, and deliberately not trusted: no convention this tool can check.
	".c":    {name: "C"},
	".h":    {name: "C"},
	".cc":   {name: "C++"},
	".cpp":  {name: "C++"},
	".hpp":  {name: "C++"},
	".m":    {name: "Objective-C"},
	".sql":  {name: "SQL"},
	".sh":   {name: "Shell"},
	".bash": {name: "Shell"},
	".lua":  {name: "Lua"},
	".pl":   {name: "Perl"},
	".hs":   {name: "Haskell"},
	".erl":  {name: "Erlang"},
	".clj":  {name: "Clojure"},
	".jl":   {name: "Julia"},
	".r":    {name: "R"},
	".tf":   {name: "Terraform"},
}

// isTestPath is the file-and-path half of the rules: names and directories that
// mean "test" in some language, plus whatever globs the config added.
func isTestPath(p string, globs []string) bool {
	base := path.Base(p)
	lower := strings.ToLower(p)
	segments := strings.Split(lower, "/")

	// A path segment that is literally a test directory. This is the rule that
	// covers most languages at once, including ones the table above does not
	// name.
	for _, seg := range segments[:max(0, len(segments)-1)] {
		switch seg {
		case "test", "tests", "spec", "specs", "__tests__", "testdata":
			return true
		}
		if strings.HasSuffix(seg, ".tests") { // C#: Thing.Tests/
			return true
		}
	}

	switch {
	case strings.HasSuffix(base, "_test.go"): // Go
		return true
	case strings.HasSuffix(lower, "_test.py"), strings.HasPrefix(strings.ToLower(base), "test_") && strings.HasSuffix(lower, ".py"):
		return true
	case strings.HasSuffix(lower, "_spec.rb"), strings.HasSuffix(lower, "_test.rb"):
		return true
	case strings.HasSuffix(lower, "tests.cs"), strings.HasSuffix(lower, "test.cs"):
		return true
	case strings.HasSuffix(lower, "tests.swift"):
		return true
	case strings.HasSuffix(lower, "test.php"):
		return true
	case strings.HasSuffix(lower, "_test.exs"):
		return true
	}

	// JS and TS: name.test.ts, name.spec.tsx, and the same for js.
	if ext := path.Ext(lower); ext != "" {
		stem := strings.TrimSuffix(lower, ext)
		if strings.HasSuffix(stem, ".test") || strings.HasSuffix(stem, ".spec") {
			return true
		}
	}

	for _, g := range globs {
		if matchGlob(g, p) {
			return true
		}
	}
	return false
}

// matchGlob is filepath.Match with one addition: a pattern containing "/"
// is matched against the whole path, and one without it against the base name,
// which is what people mean when they write `tests: ["*_check.rb"]`.
func matchGlob(pattern, p string) bool {
	if strings.Contains(pattern, "/") {
		if ok, _ := filepath.Match(pattern, p); ok {
			return true
		}
		// A directory prefix such as "qa/" or "qa/**" should match everything
		// under it.
		prefix := strings.TrimSuffix(strings.TrimSuffix(pattern, "**"), "/")
		return prefix != "" && !strings.ContainsAny(prefix, "*?[") && strings.HasPrefix(p, prefix+"/")
	}
	ok, _ := filepath.Match(pattern, path.Base(p))
	return ok
}

// testState decides the fence for one directory, given every live file under it
// — not just the files at its own level, because a test in a subdirectory still
// tests this code.
func testState(root string, files []string, cfg Config) TestState {
	var code []string
	counts := map[string]int{}

	for _, p := range files {
		if isTestPath(p, cfg.Tests) {
			return TestsFound
		}
		if lang, ok := languages[strings.ToLower(path.Ext(p))]; ok {
			code = append(code, p)
			counts[lang.name]++
		}
	}

	// Docs, configuration, SQL migrations, images. There is no code here to
	// test, so there is nothing to accuse — and a directory of 87 markdown files
	// with one stray script in it is a documentation directory, not an untested
	// package. The claim only applies where the directory is mostly code.
	if len(code) == 0 || len(code)*3 < len(files) {
		return TestsUnknown
	}

	// The cheap content scan, for languages that hide their tests in the source
	// file. A Rust module with #[cfg(test)] is tested and has no test file.
	for _, p := range code {
		lang := languages[strings.ToLower(path.Ext(p))]
		if lang.inline != "" && fileContains(filepath.Join(root, p), lang.inline, cfg.MaxScanBytes) {
			return TestsFound
		}
	}

	// Judge by the language most of the directory is written in. One stray .c
	// file should not make a Go package unknowable, and one stray .go file
	// should not let this tool pass judgement on a C project.
	best, bestN := "", 0
	for name, n := range counts {
		if n > bestN || (n == bestN && name < best) {
			best, bestN = name, n
		}
	}
	for _, lang := range languages {
		if lang.name == best {
			if lang.reliable {
				return TestsMissing
			}
			return TestsUnknown
		}
	}
	return TestsUnknown
}

func fileContains(path, needle string, limit int64) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() > limit {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// The zig marker starts with a newline so it only matches at the start of a
	// line; a file whose very first line is a test would otherwise be missed.
	return bytes.Contains(data, []byte(needle)) ||
		(strings.HasPrefix(needle, "\n") && bytes.HasPrefix(data, []byte(needle[1:])))
}
