package settings

import "testing"

// TestDefaultsAreOpen locks the fail-open posture: a site with nothing stored
// (and, by the same path, one whose settings read just failed) has registration
// open, ratings on, no maintenance and no banner. A database blip must not
// silently close signups or halt play.
func TestDefaultsAreOpen(t *testing.T) {
	d := resolve(nil)
	if !d.RegistrationOpen {
		t.Error("registration closed by default")
	}
	if !d.RatedEnabled {
		t.Error("ratings disabled by default")
	}
	if d.Maintenance {
		t.Error("maintenance on by default")
	}
	if d.NoticeText != "" {
		t.Errorf("default notice = %q, want empty", d.NoticeText)
	}
	if d.NoticeLevel != LevelInfo {
		t.Errorf("default level = %q, want %q", d.NoticeLevel, LevelInfo)
	}
}

// TestResolveOverrides covers each switch being explicitly set.
func TestResolveOverrides(t *testing.T) {
	s := resolve(map[string]string{
		KeyNoticeText:   "back in five minutes",
		KeyNoticeLevel:  LevelWarn,
		KeyRegistration: "0",
		KeyRated:        "0",
		KeyMaintenance:  "1",
	})
	if s.NoticeText != "back in five minutes" {
		t.Errorf("notice = %q", s.NoticeText)
	}
	if s.NoticeLevel != LevelWarn {
		t.Errorf("level = %q, want warn", s.NoticeLevel)
	}
	if s.RegistrationOpen || s.RatedEnabled || !s.Maintenance {
		t.Errorf("flags not applied: %+v", s)
	}
}

// TestMalformedFlagsFailClosed: only "1" is true. A garbage value in the
// "enabled" switches therefore reads as off, which is the safe direction — the
// value could only have got there by hand, and guessing "probably enabled" is
// how a paused site quietly un-pauses itself.
func TestMalformedFlagsFailClosed(t *testing.T) {
	for _, v := range []string{"", "true", "yes", "TRUE", "2", " 1"} {
		s := resolve(map[string]string{KeyRegistration: v, KeyRated: v})
		if s.RegistrationOpen {
			t.Errorf("registration open for stored value %q", v)
		}
		if s.RatedEnabled {
			t.Errorf("rated enabled for stored value %q", v)
		}
	}
	// ...and maintenance, whose safe direction is the opposite, stays off
	for _, v := range []string{"", "true", "yes"} {
		if resolve(map[string]string{KeyMaintenance: v}).Maintenance {
			t.Errorf("maintenance on for stored value %q", v)
		}
	}
}

// TestUnknownNoticeLevelReadsAsInfo: an unrecognized level must not lose the
// banner, only its styling.
func TestUnknownNoticeLevelReadsAsInfo(t *testing.T) {
	s := resolve(map[string]string{KeyNoticeText: "hello", KeyNoticeLevel: "danger"})
	if s.NoticeText != "hello" {
		t.Error("notice text lost")
	}
	if s.NoticeLevel != LevelInfo {
		t.Errorf("level = %q, want %q", s.NoticeLevel, LevelInfo)
	}
}

// TestFlagRoundTrip: Flag writes what truthy reads.
func TestFlagRoundTrip(t *testing.T) {
	if !truthy(Flag(true)) {
		t.Error("Flag(true) does not read as true")
	}
	if truthy(Flag(false)) {
		t.Error("Flag(false) reads as true")
	}
}
