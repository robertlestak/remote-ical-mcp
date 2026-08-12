package ical

import (
	"fmt"
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
)

// wrap builds a minimal VCALENDAR around the supplied component bodies.
func wrap(body ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" +
		strings.Join(body, "") + "END:VCALENDAR\r\n")
}

func event(lines ...string) string {
	return "BEGIN:VEVENT\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\n"
}

func mustParse(t *testing.T, data []byte, start, end string) []Event {
	t.Helper()
	var sp, ep *time.Time
	if start != "" {
		s, err := time.Parse(time.RFC3339, start)
		if err != nil {
			t.Fatalf("bad test start %q: %v", start, err)
		}
		sp = &s
	}
	if end != "" {
		e, err := time.Parse(time.RFC3339, end)
		if err != nil {
			t.Fatalf("bad test end %q: %v", end, err)
		}
		ep = &e
	}
	events, err := Parse(data, sp, ep)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return events
}

func starts(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Start.UTC().Format(time.RFC3339)
	}
	return out
}

// vtimezone builds the component Exchange ships alongside its Windows zone
// names. Rules are dated to 1601 and the offsets given in the ±HHMM form, both
// verbatim as Exchange writes them.
func vtimezoneFor(tzid, stdOffset, stdRule, dstOffset, dstRule string) string {
	out := "BEGIN:VTIMEZONE\r\nTZID:" + tzid + "\r\n" +
		"BEGIN:STANDARD\r\nDTSTART:16010101T020000\r\n" +
		"TZOFFSETFROM:" + stdOffset + "\r\nTZOFFSETTO:" + stdOffset + "\r\n"
	if stdRule != "" {
		out += "RRULE:" + stdRule + "\r\n"
	}
	out += "END:STANDARD\r\n"
	if dstOffset != "" {
		out += "BEGIN:DAYLIGHT\r\nDTSTART:16010101T020000\r\n" +
			"TZOFFSETFROM:" + stdOffset + "\r\nTZOFFSETTO:" + dstOffset + "\r\n"
		if dstRule != "" {
			out += "RRULE:" + dstRule + "\r\n"
		}
		out += "END:DAYLIGHT\r\n"
	}
	return out + "END:VTIMEZONE\r\n"
}

// TestWindowsTimezone covers the bug that silently zeroed every event in feeds
// produced by Exchange, which emits Windows zone IDs rather than IANA names.
//
// The names are resolved from the VTIMEZONE the feed publishes for them rather
// than from a name table, so each case carries the definition Exchange sends.
func TestWindowsTimezone(t *testing.T) {
	usStd := "FREQ=YEARLY;INTERVAL=1;BYDAY=1SU;BYMONTH=11"
	usDst := "FREQ=YEARLY;INTERVAL=1;BYDAY=2SU;BYMONTH=3"
	euStd := "FREQ=YEARLY;INTERVAL=1;BYDAY=-1SU;BYMONTH=10"
	euDst := "FREQ=YEARLY;INTERVAL=1;BYDAY=-1SU;BYMONTH=3"

	tests := []struct {
		tzid string
		vtz  string
		want string
	}{
		{"Pacific Standard Time",
			vtimezoneFor("Pacific Standard Time", "-0800", usStd, "-0700", usDst),
			"2026-08-10T17:00:00Z"}, // PDT, UTC-7
		{"Eastern Standard Time",
			vtimezoneFor("Eastern Standard Time", "-0500", usStd, "-0400", usDst),
			"2026-08-10T14:00:00Z"}, // EDT, UTC-4
		{"India Standard Time",
			vtimezoneFor("India Standard Time", "+0530", "", "", ""),
			"2026-08-10T04:30:00Z"}, // IST, UTC+5:30, no DST
		{"GMT Standard Time",
			vtimezoneFor("GMT Standard Time", "+0000", euStd, "+0100", euDst),
			"2026-08-10T09:00:00Z"}, // BST, UTC+1
		// An IANA name needs no definition in the feed at all.
		{"America/Los_Angeles", "", "2026-08-10T17:00:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.tzid, func(t *testing.T) {
			data := wrap(tc.vtz, event(
				"UID:tz-1",
				"SUMMARY:Meeting",
				"DTSTART;TZID="+tc.tzid+":20260810T100000",
				"DTEND;TZID="+tc.tzid+":20260810T110000",
			))
			events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}
			if got := events[0].Start.UTC().Format(time.RFC3339); got != tc.want {
				t.Errorf("start = %s, want %s", got, tc.want)
			}
			if events[0].Start.IsZero() {
				t.Error("start is the zero time; timezone resolution failed")
			}
		})
	}
}

// A zone the feed neither names portably nor defines cannot be placed. The
// event still lands on roughly the right day via local time, but the guess is
// recorded rather than passed off as an answer.
func TestUnresolvableZoneIsRecorded(t *testing.T) {
	cal, err := ics.ParseCalendar(strings.NewReader(string(wrap(event(
		"UID:tz-2",
		"SUMMARY:Meeting",
		"DTSTART;TZID=Pacific Standard Time:20260810T100000",
		"DTEND;TZID=Pacific Standard Time:20260810T110000",
	)))))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	tz := newTZResolver(cal)
	if loc := tz.load("Pacific Standard Time", time.Now()); loc != nil {
		t.Errorf("load returned %v for a zone with no definition in the feed", loc)
	}
	if got := tz.Unresolved(); len(got) != 1 || got[0] != "Pacific Standard Time" {
		t.Errorf("Unresolved() = %v, want the zone that could not be placed", got)
	}
}

// TestUnknownTimezoneFallsBackToVTimezone verifies that a TZID matching neither
// IANA nor the Windows table is resolved from the feed's own VTIMEZONE.
func TestUnknownTimezoneFallsBackToVTimezone(t *testing.T) {
	vtz := "BEGIN:VTIMEZONE\r\nTZID:Totally Made Up Zone\r\n" +
		"BEGIN:STANDARD\r\nDTSTART:16010101T000000\r\nTZOFFSETFROM:+0400\r\nTZOFFSETTO:+0400\r\nEND:STANDARD\r\n" +
		"END:VTIMEZONE\r\n"
	data := wrap(vtz, event(
		"UID:vtz-1",
		"SUMMARY:Meeting",
		"DTSTART;TZID=Totally Made Up Zone:20260810T100000",
		"DTEND;TZID=Totally Made Up Zone:20260810T110000",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if got := events[0].Start.UTC().Format(time.RFC3339); got != "2026-08-10T06:00:00Z" {
		t.Errorf("start = %s, want 2026-08-10T06:00:00Z (UTC+4)", got)
	}
}

// TestVTimezoneFallbackIsDSTAware guards against reading a zone name such as
// "Pacific Standard Time" as the fixed offset -0800. The name identifies a zone
// that observes both PST and PDT, and the feed's own VTIMEZONE says so. Uses an
// unmapped TZID so resolution is forced down the VTIMEZONE fallback path.
func TestVTimezoneFallbackIsDSTAware(t *testing.T) {
	// The DST rules Exchange emits for "Pacific Standard Time", verbatim.
	vtz := "BEGIN:VTIMEZONE\r\nTZID:Unmapped Pacific\r\n" +
		"BEGIN:STANDARD\r\nDTSTART:16010101T020000\r\nTZOFFSETFROM:-0700\r\nTZOFFSETTO:-0800\r\n" +
		"RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=1SU;BYMONTH=11\r\nEND:STANDARD\r\n" +
		"BEGIN:DAYLIGHT\r\nDTSTART:16010101T020000\r\nTZOFFSETFROM:-0800\r\nTZOFFSETTO:-0700\r\n" +
		"RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=2SU;BYMONTH=3\r\nEND:DAYLIGHT\r\n" +
		"END:VTIMEZONE\r\n"

	tests := []struct {
		name       string
		dtstart    string
		want       string
		start, end string
	}{
		// Daylight runs from the 2nd Sunday in March to the 1st Sunday in November.
		{"august is daylight", "20260811T073000", "2026-08-11T14:30:00Z", "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z"},
		{"july is daylight", "20260715T073000", "2026-07-15T14:30:00Z", "2026-07-01T00:00:00Z", "2026-08-01T00:00:00Z"},
		{"december is standard", "20261215T073000", "2026-12-15T15:30:00Z", "2026-12-01T00:00:00Z", "2027-01-01T00:00:00Z"},
		{"january is standard", "20260115T073000", "2026-01-15T15:30:00Z", "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := wrap(vtz, event(
				"UID:dstfb-1",
				"SUMMARY:commute",
				"DTSTART;TZID=Unmapped Pacific:"+tc.dtstart,
				"DTEND;TZID=Unmapped Pacific:"+tc.dtstart,
			))
			events := mustParse(t, data, tc.start, tc.end)
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}
			if got := events[0].Start.UTC().Format(time.RFC3339); got != tc.want {
				t.Errorf("start = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestRecurringExpansion covers the bug where an RRULE series appeared only at
// its original DTSTART, so weekly meetings vanished from any later window.
func TestRecurringExpansion(t *testing.T) {
	data := wrap(event(
		"UID:weekly-1",
		"SUMMARY:Standup",
		"DTSTART;TZID=Pacific Standard Time:20260713T100000",
		"DTEND;TZID=Pacific Standard Time:20260713T103000",
		"RRULE:FREQ=WEEKLY;BYDAY=MO",
	))
	events := mustParse(t, data, "2026-08-10T00:00:00Z", "2026-08-31T00:00:00Z")
	want := []string{
		"2026-08-10T17:00:00Z",
		"2026-08-17T17:00:00Z",
		"2026-08-24T17:00:00Z",
	}
	got := starts(events)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
	for _, e := range events {
		if !e.Recurring {
			t.Errorf("occurrence %s not flagged as recurring", e.Start)
		}
		if e.End.Sub(e.Start) != 30*time.Minute {
			t.Errorf("duration = %v, want 30m", e.End.Sub(e.Start))
		}
	}
}

// TestRecurringAcrossDSTKeepsWallClock checks that a 10:00 local meeting stays
// at 10:00 local after the DST transition rather than drifting an hour.
func TestRecurringAcrossDSTKeepsWallClock(t *testing.T) {
	data := wrap(event(
		"UID:dst-1",
		"SUMMARY:Standup",
		"DTSTART;TZID=Pacific Standard Time:20261026T100000",
		"DTEND;TZID=Pacific Standard Time:20261026T103000",
		"RRULE:FREQ=WEEKLY;BYDAY=MO",
	))
	events := mustParse(t, data, "2026-10-20T00:00:00Z", "2026-11-16T00:00:00Z")
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	if len(events) == 0 {
		t.Fatal("no occurrences")
	}
	for _, e := range events {
		if h, m, _ := e.Start.In(loc).Clock(); h != 10 || m != 0 {
			t.Errorf("occurrence %s is %02d:%02d local, want 10:00", e.Start.UTC(), h, m)
		}
	}
	// US DST ended 2026-11-01, so the offset must actually differ across it.
	first := events[0].Start.UTC().Format("15:04")
	last := events[len(events)-1].Start.UTC().Format("15:04")
	if first == last {
		t.Errorf("UTC time %s unchanged across the DST boundary", first)
	}
}

func TestExdateExcludesOccurrence(t *testing.T) {
	data := wrap(event(
		"UID:ex-1",
		"SUMMARY:Standup",
		"DTSTART;TZID=Pacific Standard Time:20260803T100000",
		"DTEND;TZID=Pacific Standard Time:20260803T103000",
		"RRULE:FREQ=WEEKLY;BYDAY=MO",
		"EXDATE;TZID=Pacific Standard Time:20260810T100000",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-08-25T00:00:00Z")
	for _, e := range events {
		if e.Start.UTC().Format(time.RFC3339) == "2026-08-10T17:00:00Z" {
			t.Error("EXDATE occurrence was not excluded")
		}
	}
	if len(events) != 3 {
		t.Errorf("got %d occurrences, want 3 (Aug 3, 17, 24)", len(events))
	}
}

// TestRecurrenceIDOverride checks that a modified instance replaces the
// generated one rather than appearing alongside it as a duplicate.
func TestRecurrenceIDOverride(t *testing.T) {
	data := wrap(
		event(
			"UID:ov-1",
			"SUMMARY:Standup",
			"DTSTART;TZID=Pacific Standard Time:20260803T100000",
			"DTEND;TZID=Pacific Standard Time:20260803T103000",
			"RRULE:FREQ=WEEKLY;BYDAY=MO",
		),
		event(
			"UID:ov-1",
			"SUMMARY:Standup (moved)",
			"RECURRENCE-ID;TZID=Pacific Standard Time:20260810T100000",
			"DTSTART;TZID=Pacific Standard Time:20260810T140000",
			"DTEND;TZID=Pacific Standard Time:20260810T143000",
		),
	)
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-08-18T00:00:00Z")

	var onAug10 []Event
	for _, e := range events {
		if e.Start.UTC().Format("2006-01-02") == "2026-08-10" {
			onAug10 = append(onAug10, e)
		}
	}
	if len(onAug10) != 1 {
		t.Fatalf("got %d events on Aug 10, want exactly 1 (the override)", len(onAug10))
	}
	if onAug10[0].Summary != "Standup (moved)" {
		t.Errorf("summary = %q, want the override", onAug10[0].Summary)
	}
	if got := onAug10[0].Start.UTC().Format(time.RFC3339); got != "2026-08-10T21:00:00Z" {
		t.Errorf("start = %s, want the override time 2026-08-10T21:00:00Z", got)
	}
}

// TestOverlapFiltering covers the bug where the range filter compared only the
// start time, dropping events already in progress at the window boundary.
func TestOverlapFiltering(t *testing.T) {
	data := wrap(event(
		"UID:long-1",
		"SUMMARY:All-hands",
		"DTSTART:20260810T080000Z",
		"DTEND:20260810T170000Z",
	))
	tests := []struct {
		name       string
		start, end string
		want       int
	}{
		{"window starts mid-event", "2026-08-10T12:00:00Z", "2026-08-10T13:00:00Z", 1},
		{"window ends mid-event", "2026-08-10T06:00:00Z", "2026-08-10T09:00:00Z", 1},
		{"window inside event", "2026-08-10T10:00:00Z", "2026-08-10T11:00:00Z", 1},
		{"window entirely before", "2026-08-09T00:00:00Z", "2026-08-10T08:00:00Z", 0},
		{"window entirely after", "2026-08-10T17:00:00Z", "2026-08-11T00:00:00Z", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(mustParse(t, data, tc.start, tc.end)); got != tc.want {
				t.Errorf("got %d events, want %d", got, tc.want)
			}
		})
	}
}

func TestAllDayEvent(t *testing.T) {
	data := wrap(event(
		"UID:ad-1",
		"SUMMARY:Conference",
		"DTSTART;VALUE=DATE:20260824",
		"DTEND;VALUE=DATE:20260827",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !events[0].AllDay {
		t.Error("AllDay = false, want true")
	}
	if d := events[0].End.Sub(events[0].Start); d != 72*time.Hour {
		t.Errorf("duration = %v, want 72h", d)
	}
}

func TestDurationProperty(t *testing.T) {
	data := wrap(event(
		"UID:dur-1",
		"SUMMARY:Workshop",
		"DTSTART:20260810T090000Z",
		"DURATION:PT1H30M",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if d := events[0].End.Sub(events[0].Start); d != 90*time.Minute {
		t.Errorf("duration = %v, want 1h30m", d)
	}
}

func TestParseICalDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
		bad  bool
	}{
		{"PT1H", time.Hour, false},
		{"PT30M", 30 * time.Minute, false},
		{"PT1H30M", 90 * time.Minute, false},
		{"P1D", 24 * time.Hour, false},
		{"P2DT3H", 51 * time.Hour, false},
		{"P1W", 7 * 24 * time.Hour, false},
		{"PT45S", 45 * time.Second, false},
		{"-PT1H", -time.Hour, false},
		{"garbage", 0, true},
	}
	for _, tc := range tests {
		got, err := parseICalDuration(tc.in)
		if tc.bad {
			if err == nil {
				t.Errorf("parseICalDuration(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseICalDuration(%q): %v", tc.in, err)
		} else if got != tc.want {
			t.Errorf("parseICalDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseUTCOffset(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"+0000", 0},
		{"-0800", -8 * 3600},
		{"+0530", 5*3600 + 30*60},
		{"-0330", -(3*3600 + 30*60)},
		{"+053045", 5*3600 + 30*60 + 45},
	}
	for _, tc := range tests {
		got, err := parseUTCOffset(tc.in)
		if err != nil {
			t.Errorf("parseUTCOffset(%q): %v", tc.in, err)
		} else if got != tc.want {
			t.Errorf("parseUTCOffset(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestCategoriesAreSplit checks that a comma-separated CATEGORIES value becomes
// separate entries rather than one combined string.
func TestCategoriesAreSplit(t *testing.T) {
	data := wrap(event(
		"UID:cat-1",
		"SUMMARY:Tagged",
		"DTSTART:20260810T090000Z",
		"DTEND:20260810T100000Z",
		"CATEGORIES:Work,Important",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if got := events[0].Categories; len(got) != 2 || got[0] != "Work" || got[1] != "Important" {
		t.Errorf("categories = %v, want [Work Important]", got)
	}
}

// TestMalformedEventDoesNotDropCalendar checks that one bad VEVENT does not
// discard the events around it.
func TestMalformedEventDoesNotDropCalendar(t *testing.T) {
	data := wrap(
		event("UID:bad-1", "SUMMARY:No start"),
		event("UID:good-1", "SUMMARY:Fine", "DTSTART:20260810T090000Z", "DTEND:20260810T100000Z"),
	)
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
	if len(events) != 1 || events[0].UID != "good-1" {
		t.Errorf("got %v, want just the well-formed event", starts(events))
	}
}

func TestResultsAreSortedByStart(t *testing.T) {
	data := wrap(
		event("UID:c", "SUMMARY:C", "DTSTART:20260810T150000Z", "DTEND:20260810T160000Z"),
		event("UID:a", "SUMMARY:A", "DTSTART:20260810T090000Z", "DTEND:20260810T100000Z"),
		event("UID:b", "SUMMARY:B", "DTSTART:20260810T120000Z", "DTEND:20260810T130000Z"),
	)
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
	for i := 1; i < len(events); i++ {
		if events[i].Start.Before(events[i-1].Start) {
			t.Fatalf("events out of order: %v", starts(events))
		}
	}
}

func TestEmptyResultIsEmptySlice(t *testing.T) {
	data := wrap(event("UID:x", "SUMMARY:X", "DTSTART:20200101T090000Z", "DTEND:20200101T100000Z"))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
	if events == nil {
		t.Error("events is nil; it must marshal as [] rather than null")
	}
	if len(events) != 0 {
		t.Errorf("got %d events, want 0", len(events))
	}
}

func TestInvertedWindowIsRejected(t *testing.T) {
	data := wrap(event("UID:x", "SUMMARY:X", "DTSTART:20260810T090000Z", "DTEND:20260810T100000Z"))
	start, _ := time.Parse(time.RFC3339, "2026-09-01T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2026-08-01T00:00:00Z")
	if _, err := Parse(data, &start, &end); err == nil {
		t.Error("expected an error when end_date precedes start_date")
	}
}
