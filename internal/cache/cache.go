// Package cache keeps the parsed repository next to the repository itself.
//
// Reading 50,000 commits takes seconds, and nothing about them changes until
// somebody commits again. The key is the HEAD SHA plus every option that
// changes what gets parsed, so a stale entry cannot be returned: a different
// HEAD, or a different --since, is a different file.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// version is part of every key. Bumping it retires every cache file in the
// world at once, which is what a change to the classification rules needs.
const version = "v1"

// Dir is where entries live: inside .git, so they are invisible to git status,
// never committed, and deleted along with the clone.
func Dir(gitDir string) string { return filepath.Join(gitDir, "git-farm") }

// Key hashes the head SHA together with everything else that would change the
// answer.
func Key(head string, parts ...string) string {
	sum := sha256.Sum256([]byte(version + "\x00" + head + "\x00" + strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// Load reads an entry into v. A miss is not an error worth reporting: it just
// returns false, and the caller does the work.
func Load(gitDir, key string, v any) bool {
	data, err := os.ReadFile(filepath.Join(Dir(gitDir), key+".json"))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

// Save writes an entry, and drops the ones this repository will never ask for
// again — every commit makes a new key, so without a sweep the directory would
// grow one file per commit forever.
func Save(gitDir, key string, v any) error {
	dir := Dir(gitDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	// Write to a temporary file and rename, so a cancelled run cannot leave
	// half a JSON document behind for the next one to trip over.
	tmp, err := os.CreateTemp(dir, "tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, key+".json")); err != nil {
		return err
	}

	sweep(dir, key)
	return nil
}

// sweep keeps the newest few entries. A handful is enough to cover switching
// between two branches, or between two --since values, without a growing pile.
func sweep(dir, keep string) {
	const maxEntries = 6

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= maxEntries {
		return
	}

	type aged struct {
		name string
		mod  int64
	}
	var files []aged
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == keep+".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, aged{e.Name(), info.ModTime().UnixNano()})
	}

	for len(files) >= maxEntries {
		oldest := 0
		for i, f := range files {
			if f.mod < files[oldest].mod {
				oldest = i
			}
		}
		os.Remove(filepath.Join(dir, files[oldest].name))
		files = append(files[:oldest], files[oldest+1:]...)
	}
}
