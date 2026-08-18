package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTestPath(t *testing.T) {
	yes := []string{
		"internal/store/db_test.go",
		"src/components/Button.test.tsx",
		"src/components/Button.spec.js",
		"app/__tests__/render.js",
		"tests/test_parser.py",
		"pkg/parser_test.py",
		"spec/models/user_spec.rb",
		"src/test/java/com/example/AppTest.java",
		"Thing.Tests/ThingTests.cs",
		"Sources/AppTests.swift",
		"tests/helpers/fixtures.rb",
	}
	for _, p := range yes {
		if !isTestPath(p, nil) {
			t.Errorf("%s should be recognised as a test", p)
		}
	}

	no := []string{
		"internal/store/db.go",
		"src/components/Button.tsx",
		"cmd/tool/main.go",
		"docs/testing.md",     // a document about testing is not a test
		"internal/contest.go", // "test" inside a word is not a test either
		"protest/main.go",
	}
	for _, p := range no {
		if isTestPath(p, nil) {
			t.Errorf("%s should not be recognised as a test", p)
		}
	}
}

func TestTestGlobOverride(t *testing.T) {
	globs := []string{"*_check.rb", "qa/"}

	for _, p := range []string{"lib/user_check.rb", "qa/smoke/login.rb"} {
		if !isTestPath(p, globs) {
			t.Errorf("%s should match the configured globs %v", p, globs)
		}
	}
	if isTestPath("lib/user.rb", globs) {
		t.Error("an ordinary file matched the configured globs")
	}
}

// The three states, and the reason the third one exists.
func TestTestState(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Rust puts its tests inside the file it tests. Calling this untested would
	// be a false accusation printed on somebody's README.
	write("rust/tested/lib.rs", "pub fn add() {}\n\n#[cfg(test)]\nmod tests {\n  #[test]\n  fn works() {}\n}\n")
	write("rust/bare/lib.rs", "pub fn add() {}\n")

	cfg := Defaults()
	cases := []struct {
		name  string
		files []string
		want  TestState
	}{
		{"go with a test file", []string{"internal/ui/view.go", "internal/ui/view_test.go"}, TestsFound},
		{"go without one", []string{"cmd/tool/main.go"}, TestsMissing},
		{"a test in a subdirectory still counts", []string{"pkg/a.go", "pkg/deep/a_test.go"}, TestsFound},

		{"rust with inline tests", []string{"rust/tested/lib.rs"}, TestsFound},
		{"rust with none", []string{"rust/bare/lib.rs"}, TestsMissing},

		// Nothing here is code, so there is nothing to accuse.
		{"sql migrations", []string{"migrations/0001_init.sql", "migrations/0002_x.sql"}, TestsUnknown},
		{"documentation", []string{"docs/a.md", "docs/b.md", "docs/c.md"}, TestsUnknown},

		// A language whose convention this tool has not checked.
		{"c", []string{"src/main.c", "src/util.c", "src/util.h"}, TestsUnknown},

		// One stray script in a directory of documents is a documentation
		// directory with a script in it, not an untested package.
		{"a script among the docs", []string{"docs/a.md", "docs/b.md", "docs/c.md", "docs/d.md", "docs/build.js"}, TestsUnknown},

		{"nothing at all", nil, TestsUnknown},
	}

	for _, c := range cases {
		if got := testState(root, c.files, cfg); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// An inline marker must not be found in a file that only mentions it.
func TestInlineScanReadsRealFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "lib.rs"), []byte("pub fn add() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if fileContains(filepath.Join(root, "lib.rs"), "#[cfg(test)]", 1<<20) {
		t.Error("found a marker that is not in the file")
	}
	if fileContains(filepath.Join(root, "missing.rs"), "#[cfg(test)]", 1<<20) {
		t.Error("found a marker in a file that does not exist")
	}
}
