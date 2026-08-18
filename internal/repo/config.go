package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every number that decides what the farm looks like.
//
// They are all in one struct on purpose. These thresholds will be changed a
// dozen times while looking at real repositories, and a threshold buried in an
// if-statement somewhere is a threshold nobody dares to touch.
//
// The percentiles are the important idea. A fixed rule — "more than 40 commits
// is a hotspot" — makes a small repo look immaculate and a large one look like
// a disaster. Ranking within the repo instead means the picture always has some
// weeds and some calm, whatever the size.
type Config struct {
	// Depth is how many path segments a field name keeps: internal/store/x/y.go
	// at depth 2 belongs to the field internal/store/.
	Depth int `json:"depth"`

	// BigFiles is the share of files that count as big. 0.10 means the top 10%
	// by line count are drawn as tall plants.
	BigFiles float64 `json:"big_files"`

	// Churn is the share of files that count as weeds, ranked by
	// commits × distinct authors. A file changed constantly by many people is
	// where the bugs are; on the farm it is just the ugly corner.
	Churn float64 `json:"churn"`

	// DirChurn is the share of directories that become hotspots, ranked by
	// their median file churn.
	DirChurn float64 `json:"dir_churn"`

	// HotspotFiles is how many live files a directory needs before it can be
	// called a hotspot. A median over one file is that file, and one busy file
	// is not a busy directory — without this, every single-file directory in a
	// large repo outranks the code.
	HotspotFiles int `json:"hotspot_files"`

	// Quiet is how long a file must go untouched before it is dead code.
	Quiet Span `json:"quiet"`

	// Tests are extra globs that mark a path as a test, for repos whose layout
	// none of the built-in rules knows about. A directory containing a match is
	// fenced.
	Tests []string `json:"tests"`

	// MaxScanBytes caps the files whose contents are read when looking for
	// inline tests. Nothing useful is inline in a 2 MB generated file.
	MaxScanBytes int64 `json:"max_scan_bytes"`
}

// Defaults are the thresholds the plan settled on. Everything is overridable
// because none of these numbers is more than a first guess.
func Defaults() Config {
	return Config{
		Depth:        2,
		BigFiles:     0.10,
		Churn:        0.10,
		DirChurn:     0.20,
		HotspotFiles: 3,
		Quiet:        Span(365 * 24 * time.Hour),
		MaxScanBytes: 512 * 1024,
	}
}

// LoadConfig reads a JSON config file over the defaults. A missing file is not
// an error: it is the normal case.
func LoadConfig(path string) (Config, error) {
	cfg := Defaults()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	// Unmarshalling onto the defaults leaves any field the file omits alone, so
	// a config can set one threshold without restating the rest.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg.normalise(), nil
}

func (c Config) normalise() Config {
	if c.Depth < 1 {
		c.Depth = 1
	}
	clamp := func(v, def float64) float64 {
		if v <= 0 || v >= 1 {
			return def
		}
		return v
	}
	c.BigFiles = clamp(c.BigFiles, 0.10)
	c.Churn = clamp(c.Churn, 0.10)
	c.DirChurn = clamp(c.DirChurn, 0.20)
	if c.HotspotFiles < 1 {
		c.HotspotFiles = 1
	}
	if c.Quiet <= 0 {
		c.Quiet = Span(365 * 24 * time.Hour)
	}
	if c.MaxScanBytes <= 0 {
		c.MaxScanBytes = 512 * 1024
	}
	return c
}

// A Span is a duration written the way people write repository ages: 5y, 18m,
// 30d. time.ParseDuration stops at hours, which is no use for a tool whose
// shortest interesting unit is a day.
type Span time.Duration

func (s Span) Duration() time.Duration { return time.Duration(s) }

func (s Span) String() string {
	d := time.Duration(s)
	switch {
	case d%(365*24*time.Hour) == 0:
		return strconv.Itoa(int(d/(365*24*time.Hour))) + "y"
	case d%(24*time.Hour) == 0:
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	default:
		return d.String()
	}
}

func (s Span) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

func (s *Span) UnmarshalJSON(b []byte) error {
	var text string
	if err := json.Unmarshal(b, &text); err != nil {
		return err
	}
	v, err := ParseSpan(text)
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// ParseSpan reads 5y, 18m, 30d, 12h, or anything time.ParseDuration takes.
func ParseSpan(text string) (Span, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("empty duration")
	}

	units := map[byte]time.Duration{
		'y': 365 * 24 * time.Hour,
		'm': 30 * 24 * time.Hour, // a month, not a minute: this tool has no minutes
		'w': 7 * 24 * time.Hour,
		'd': 24 * time.Hour,
	}

	if unit, ok := units[text[len(text)-1]]; ok {
		n, err := strconv.ParseFloat(text[:len(text)-1], 64)
		if err != nil {
			return 0, fmt.Errorf("bad duration %q: try 5y, 18m, 30d", text)
		}
		if n <= 0 {
			return 0, fmt.Errorf("duration %q must be positive", text)
		}
		return Span(time.Duration(n * float64(unit))), nil
	}

	d, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("bad duration %q: try 5y, 18m, 30d", text)
	}
	return Span(d), nil
}
