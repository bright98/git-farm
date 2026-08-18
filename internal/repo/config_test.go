package repo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSpan(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"5y", 5 * 365 * 24 * time.Hour},
		{"1y", 365 * 24 * time.Hour},
		{"18m", 18 * 30 * 24 * time.Hour}, // m is months: this tool has no minutes
		{"2w", 14 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"0.5y", 182*24*time.Hour + 12*time.Hour},
		{"48h", 48 * time.Hour},
	}
	for _, c := range cases {
		got, err := ParseSpan(c.in)
		if err != nil {
			t.Errorf("ParseSpan(%q): %v", c.in, err)
			continue
		}
		if got.Duration() != c.want {
			t.Errorf("ParseSpan(%q) = %v, want %v", c.in, got.Duration(), c.want)
		}
	}

	for _, bad := range []string{"", "soon", "-3d", "0y", "5 years"} {
		if _, err := ParseSpan(bad); err == nil {
			t.Errorf("ParseSpan(%q) should have failed", bad)
		}
	}
}

// A config file sets one threshold without having to restate the rest.
func TestLoadConfigOverlaysDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"quiet": "18m", "tests": ["qa/"], "depth": 3}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Depth != 3 {
		t.Errorf("depth = %d, want 3", cfg.Depth)
	}
	if want := 18 * 30 * 24 * time.Hour; cfg.Quiet.Duration() != want {
		t.Errorf("quiet = %v, want %v", cfg.Quiet.Duration(), want)
	}
	if len(cfg.Tests) != 1 || cfg.Tests[0] != "qa/" {
		t.Errorf("tests = %v", cfg.Tests)
	}
	// Untouched by the file, so still the default.
	if cfg.Churn != Defaults().Churn {
		t.Errorf("churn = %v, want the default %v", cfg.Churn, Defaults().Churn)
	}
}

// A missing config file is the normal case, not an error.
func TestLoadConfigMissingFile(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "nothing.json"))
	if err != nil {
		t.Fatalf("a missing config file must not be an error: %v", err)
	}
	def := Defaults()
	if cfg.Depth != def.Depth || cfg.Churn != def.Churn || cfg.Quiet != def.Quiet {
		t.Errorf("a missing config file changed the defaults: %+v", cfg)
	}
}

// Nonsense in a config file must not produce a nonsense farm.
func TestConfigNormalise(t *testing.T) {
	cfg := Config{Depth: 0, BigFiles: 4, Churn: -1, DirChurn: 0, HotspotFiles: 0}.normalise()

	if cfg.Depth != 1 {
		t.Errorf("depth = %d, want at least 1", cfg.Depth)
	}
	if cfg.BigFiles != Defaults().BigFiles || cfg.Churn != Defaults().Churn || cfg.DirChurn != Defaults().DirChurn {
		t.Errorf("a share outside 0..1 was kept: %+v", cfg)
	}
	if cfg.HotspotFiles < 1 || cfg.Quiet <= 0 || cfg.MaxScanBytes <= 0 {
		t.Errorf("normalise left an unusable value: %+v", cfg)
	}
}

// The config travels into the cache key as JSON, so it has to survive the trip.
func TestSpanRoundTrip(t *testing.T) {
	for _, text := range []string{"5y", "30d"} {
		span, err := ParseSpan(text)
		if err != nil {
			t.Fatal(err)
		}
		if span.String() != text {
			t.Errorf("%q became %q", text, span.String())
		}

		data, err := span.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		var back Span
		if err := back.UnmarshalJSON(data); err != nil {
			t.Fatal(err)
		}
		if back != span {
			t.Errorf("%q round-tripped to %v", text, back.Duration())
		}
	}
}
