package main

import (
	"testing"
	"time"

	"github.com/bright98/git-farm/internal/repo"
)

func TestNightModeParses(t *testing.T) {
	cases := []struct {
		in   string
		want nightMode
		bad  bool
	}{
		{in: "true", want: nightOn},
		{in: "1", want: nightOn},
		{in: "on", want: nightOn},
		{in: "false", want: nightOff},
		{in: "0", want: nightOff},
		{in: "auto", want: nightAuto},
		{in: "AUTO", want: nightAuto},
		{in: "maybe", bad: true},
		{in: "", bad: true},
	}

	for _, c := range cases {
		var n nightMode
		err := n.Set(c.in)
		switch {
		case c.bad && err == nil:
			t.Errorf("--night=%q was accepted", c.in)
		case !c.bad && err != nil:
			t.Errorf("--night=%q: %v", c.in, err)
		case !c.bad && n != c.want:
			t.Errorf("--night=%q parsed as %v, want %v", c.in, n, c.want)
		}
	}
}

// `--night` on its own has to keep meaning yes. It is the form people type,
// and a flag.Value that does not claim to be boolean makes it an error that
// swallows the next argument.
func TestNightIsStillABareFlag(t *testing.T) {
	var n nightMode
	if !n.IsBoolFlag() {
		t.Error("--night no longer stands on its own")
	}
}

func TestNightAutoFollowsTheAuthorsClock(t *testing.T) {
	// +03:30 is the point of the exercise: every one of these instants is a
	// different hour in UTC, and UTC is not the clock the author looked at.
	tz := time.FixedZone("+0330", 3*3600+30*60)

	cases := []struct {
		hour int
		want bool
	}{
		{0, true},   // midnight, the moment the rule is named after
		{2, true},   // the small hours
		{5, true},   // last hour that counts
		{6, false},  // dawn
		{14, false}, // the middle of the afternoon
		{23, false}, // late, but not yet after midnight
	}

	for _, c := range cases {
		r := &repo.Repo{LastChange: time.Date(2026, 8, 20, c.hour, 14, 0, 0, tz)}
		if got := nightAuto.at(r); got != c.want {
			t.Errorf("a commit at %02d:14 +0330 draws night=%v, want %v", c.hour, got, c.want)
		}
		// The other two settings do not ask the repository anything.
		if nightOn.at(r) != true || nightOff.at(r) != false {
			t.Errorf("at %02d:14 the fixed settings moved", c.hour)
		}
	}
}

// A repository whose commits changed nothing has no clock to read, and a farm
// nobody is standing in should not be drawn at night on a guess.
func TestNightAutoWithoutACommitIsDay(t *testing.T) {
	if nightAuto.at(&repo.Repo{}) {
		t.Error("auto drew night for a repository with no change to point at")
	}
}
