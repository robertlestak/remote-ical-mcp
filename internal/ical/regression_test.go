package ical

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// unmappedPacific is the DST ruleset Exchange emits for Pacific time, under a
// TZID matching neither IANA nor the Windows table — so resolution is forced
// down the VTIMEZONE fallback, which can only ever answer with a fixed offset.
const unmappedPacific = "BEGIN:VTIMEZONE\r\nTZID:Unmapped Pacific\r\n" +
	"BEGIN:STANDARD\r\nDTSTART:16010101T020000\r\nTZOFFSETFROM:-0700\r\nTZOFFSETTO:-0800\r\n" +
	"RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=1SU;BYMONTH=11\r\nEND:STANDARD\r\n" +
	"BEGIN:DAYLIGHT\r\nDTSTART:16010101T020000\r\nTZOFFSETFROM:-0800\r\nTZOFFSETTO:-0700\r\n" +
	"RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=2SU;BYMONTH=3\r\nEND:DAYLIGHT\r\n" +
	"END:VTIMEZONE\r\n"

// A series repeats on the wall clock, so a 10:00 standup is at 10:00 either
// side of a DST transition. TestRecurringAcrossDSTKeepsWallClock covers this
// for a zone that resolves to an IANA location, where rrule repeats on that
// location's clock for free. The fallback zone has no location to repeat on:
// resolving it once freezes the offset and every occurrence past the
// transition slides an hour.
func TestRecurringInVTimezoneFallbackKeepsWallClock(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("tzdata unavailable")
	}

	data := wrap(unmappedPacific, event(
		"UID:fallback-dst-1",
		"SUMMARY:Standup",
		"DTSTART;TZID=Unmapped Pacific:20261026T100000",
		"DTEND;TZID=Unmapped Pacific:20261026T103000",
		"RRULE:FREQ=WEEKLY;BYDAY=MO",
	))
	events := mustParse(t, data, "2026-10-20T00:00:00Z", "2026-11-16T00:00:00Z")
	if len(events) < 3 {
		t.Fatalf("got %d occurrences, want the weeks either side of the transition", len(events))
	}
	for _, e := range events {
		if h, m, _ := e.Start.In(loc).Clock(); h != 10 || m != 0 {
			t.Errorf("occurrence %s is %02d:%02d local, want 10:00",
				e.Start.UTC().Format(time.RFC3339), h, m)
		}
	}
	// US DST ended 2026-11-01, so the UTC offset must actually change across
	// the window, or the assertion above proves nothing.
	if first, last := starts(events)[0], starts(events)[len(events)-1]; first[11:16] == last[11:16] {
		t.Errorf("UTC time unchanged across the DST boundary; the window missed the transition")
	}
}

// EXDATE resolves to a true instant. Once occurrences drift, the two no longer
// meet and a cancelled meeting quietly comes back — worse than a shifted one,
// because it invents an event that never happened.
func TestExdateCancelsAcrossDSTInFallbackZone(t *testing.T) {
	data := wrap(unmappedPacific, event(
		"UID:fallback-exdate-1",
		"SUMMARY:Standup",
		"DTSTART;TZID=Unmapped Pacific:20261026T100000",
		"DTEND;TZID=Unmapped Pacific:20261026T103000",
		"RRULE:FREQ=WEEKLY;BYDAY=MO",
		"EXDATE;TZID=Unmapped Pacific:20261109T100000",
	))
	events := mustParse(t, data, "2026-10-20T00:00:00Z", "2026-11-16T00:00:00Z")

	want := []string{"2026-10-26T17:00:00Z", "2026-11-02T18:00:00Z"}
	if got := starts(events); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("occurrences = %v, want %v (2026-11-09 is cancelled)", got, want)
	}
}

// UNTIL is written in UTC even when DTSTART is a local time, so a wall-clock
// expansion has to hand the bound back to the timeline before applying it. The
// error runs in opposite directions either side of UTC, so both are covered,
// with the bound placed inside the offending gap.
func TestSeriesEndsWhereUNTILSaysOnTheTimeline(t *testing.T) {
	// A zone east of UTC, where the wall clock reads later than the instant.
	unmappedIndia := "BEGIN:VTIMEZONE\r\nTZID:Unmapped India\r\n" +
		"BEGIN:STANDARD\r\nDTSTART:16010101T000000\r\nTZOFFSETFROM:+0530\r\nTZOFFSETTO:+0530\r\n" +
		"END:STANDARD\r\nEND:VTIMEZONE\r\n"

	tests := []struct {
		name  string
		vtz   string
		tzid  string
		until string
		want  []string
	}{
		{
			name:  "west of utc drops the occurrence past until",
			vtz:   unmappedPacific,
			tzid:  "Unmapped Pacific",
			until: "20261104T140000Z",
			want:  []string{"2026-10-21T17:00:00Z", "2026-10-28T17:00:00Z"},
		},
		{
			name:  "east of utc keeps the occurrence still inside until",
			vtz:   unmappedIndia,
			tzid:  "Unmapped India",
			until: "20261104T060000Z",
			want: []string{
				"2026-10-21T04:30:00Z", "2026-10-28T04:30:00Z", "2026-11-04T04:30:00Z",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := wrap(tc.vtz, event(
				"UID:until-1",
				"SUMMARY:Standup",
				"DTSTART;TZID="+tc.tzid+":20261021T100000",
				"DTEND;TZID="+tc.tzid+":20261021T103000",
				"RRULE:FREQ=WEEKLY;BYDAY=WE;UNTIL="+tc.until,
			))
			events := mustParse(t, data, "2026-10-15T00:00:00Z", "2026-11-30T00:00:00Z")
			if got := starts(events); fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("occurrences = %v, want %v", got, tc.want)
			}
		})
	}
}

// RFC 5545 §3.8.5.2: the recurrence set is DTSTART together with RRULE and
// RDATE, minus EXDATE. An event carrying RDATE alone still happens on the day
// it was written for.
func TestRdateKeepsDtstartOccurrence(t *testing.T) {
	data := wrap(event(
		"UID:rdate-1",
		"SUMMARY:Occasional",
		"DTSTART:20260810T170000Z",
		"DTEND:20260810T173000Z",
		"RDATE:20260817T170000Z,20260824T170000Z",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")

	want := []string{"2026-08-10T17:00:00Z", "2026-08-17T17:00:00Z", "2026-08-24T17:00:00Z"}
	if got := starts(events); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
}

// An RDATE naming DTSTART again must not produce the event twice.
func TestRdateDuplicatingDtstartIsDeduped(t *testing.T) {
	data := wrap(event(
		"UID:rdate-2",
		"SUMMARY:Occasional",
		"DTSTART:20260810T170000Z",
		"DTEND:20260810T173000Z",
		"RDATE:20260810T170000Z",
	))
	if events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z"); len(events) != 1 {
		t.Errorf("occurrences = %v, want one", starts(events))
	}
}

// An EXDATE cancelling DTSTART itself still applies once DTSTART is seeded into
// the recurrence set.
func TestExdateCancelsDtstartOfRdateSeries(t *testing.T) {
	data := wrap(event(
		"UID:rdate-3",
		"SUMMARY:Occasional",
		"DTSTART:20260810T170000Z",
		"DTEND:20260810T173000Z",
		"RDATE:20260817T170000Z",
		"EXDATE:20260810T170000Z",
	))
	want := []string{"2026-08-17T17:00:00Z"}
	if got := starts(mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
}

// A VTIMEZONE observance whose RRULE carries no BYMONTH takes its month and day
// from DTSTART. Re-anchoring such a rule to January moves the transition there
// and pins the zone to one offset for the rest of the year.
func TestVTimezoneRuleWithoutBymonth(t *testing.T) {
	vtz := "BEGIN:VTIMEZONE\r\nTZID:Odd Zone\r\n" +
		"BEGIN:STANDARD\r\nDTSTART:19701025T020000\r\nTZOFFSETFROM:+0200\r\nTZOFFSETTO:+0100\r\n" +
		"RRULE:FREQ=YEARLY\r\nEND:STANDARD\r\n" +
		"BEGIN:DAYLIGHT\r\nDTSTART:19700329T020000\r\nTZOFFSETFROM:+0100\r\nTZOFFSETTO:+0200\r\n" +
		"RRULE:FREQ=YEARLY\r\nEND:DAYLIGHT\r\n" +
		"END:VTIMEZONE\r\n"

	tests := []struct{ name, dtstart, want string }{
		{"june is daylight", "20260615T120000", "2026-06-15T10:00:00Z"},
		{"december is standard", "20261215T120000", "2026-12-15T11:00:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := wrap(vtz, event(
				"UID:nobymonth-1",
				"SUMMARY:Meeting",
				"DTSTART;TZID=Odd Zone:"+tc.dtstart,
				"DTEND;TZID=Odd Zone:"+tc.dtstart,
			))
			events := mustParse(t, data, "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z")
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}
			if got := events[0].Start.UTC().Format(time.RFC3339); got != tc.want {
				t.Errorf("start = %s, want %s", got, tc.want)
			}
		})
	}
}

// A calendar larger than the cap is refused rather than read into memory.
func TestFetchRejectsOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("BEGIN:VCALENDAR\r\n"))
		chunk := strings.Repeat("X", 1<<20)
		for written := 0; written <= maxCalendarBytes; written += len(chunk) {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, FetchOptions{AllowPrivateHosts: true})
	if err == nil {
		t.Fatal("oversized calendar was accepted")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Errorf("error = %v, want it to name the size limit", err)
	}
}

// A URL that arrived in a tool call is held to public addresses, so the server
// cannot be used to reach whatever it happens to be able to reach.
func TestFetchRefusesPrivateAddressesUnlessAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"))
	}))
	defer srv.Close()

	if _, err := Fetch(context.Background(), srv.URL, FetchOptions{}); err == nil {
		t.Error("a loopback address was fetched without AllowPrivateHosts")
	}

	// A configured calendar may legitimately be self-hosted.
	if _, err := Fetch(context.Background(), srv.URL, FetchOptions{AllowPrivateHosts: true}); err != nil {
		t.Errorf("configured calendar on a private address was refused: %v", err)
	}
}

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"93.184.216.34", true},
		{"2606:2800:220:1:248:1893:25c8:1946", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"10.0.0.5", false},
		{"192.168.1.10", false},
		{"172.16.0.1", false},
		// The cloud metadata endpoint, the address this guard exists for.
		{"169.254.169.254", false},
		{"fe80::1", false},
		{"fd00::1", false},
		{"0.0.0.0", false},
		{"224.0.0.1", false},
		// Carrier-grade NAT is 100.64.0.0/10, so it runs to 100.127.255.255.
		{"100.64.0.0", false},
		{"100.64.0.1", false},
		{"100.127.255.255", false},
		// Either side of that range is ordinary public space.
		{"100.63.255.255", true},
		{"100.128.0.0", true},
		{"101.0.0.1", true},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test address %q", c.ip)
		}
		if got := isPublicIP(ip); got != c.want {
			t.Errorf("isPublicIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// A cancelled tool call must not keep fetching for the full timeout.
func TestFetchHonoursContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Fetch(ctx, srv.URL, FetchOptions{AllowPrivateHosts: true}); err == nil {
		t.Error("fetch ignored a cancelled context")
	}
}

// RANGE=THISANDFUTURE is what a client writes when you choose "this and all
// following events". The edit owns its slot and every later one; reading it as
// an ordinary single-instance override leaves the rest of the series showing
// the details and the time it was moved away from.
func TestThisAndFutureGovernsLaterOccurrences(t *testing.T) {
	// Weekly Monday 10:00Z. From Aug 17 the series moves to 14:00Z and is
	// renamed; Aug 3 and Aug 10 keep the original.
	data := wrap(event(
		"UID:series",
		"SUMMARY:Standup",
		"DTSTART:20260803T100000Z",
		"DTEND:20260803T103000Z",
		"RRULE:FREQ=WEEKLY;BYDAY=MO",
	), event(
		"UID:series",
		"SUMMARY:Standup (afternoon)",
		"RECURRENCE-ID;RANGE=THISANDFUTURE:20260817T100000Z",
		"DTSTART:20260817T140000Z",
		"DTEND:20260817T143000Z",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")

	want := []struct{ start, summary string }{
		{"2026-08-03T10:00:00Z", "Standup"},
		{"2026-08-10T10:00:00Z", "Standup"},
		{"2026-08-17T14:00:00Z", "Standup (afternoon)"},
		{"2026-08-24T14:00:00Z", "Standup (afternoon)"},
		{"2026-08-31T14:00:00Z", "Standup (afternoon)"},
	}
	if len(events) != len(want) {
		t.Fatalf("occurrences = %v, want %d", starts(events), len(want))
	}
	for i, w := range want {
		if got := events[i].Start.UTC().Format(time.RFC3339); got != w.start {
			t.Errorf("occurrence %d starts %s, want %s", i, got, w.start)
		}
		if events[i].Summary != w.summary {
			t.Errorf("occurrence %d is %q, want %q", i, events[i].Summary, w.summary)
		}
	}
}

// A single-instance override inside the range still wins: it names one slot
// exactly, which is more specific than an edit that governs a span.
func TestSingleOverrideBeatsThisAndFuture(t *testing.T) {
	data := wrap(event(
		"UID:series",
		"SUMMARY:Standup",
		"DTSTART:20260803T100000Z",
		"DTEND:20260803T103000Z",
		"RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=4",
	), event(
		"UID:series",
		"SUMMARY:Standup (afternoon)",
		"RECURRENCE-ID;RANGE=THISANDFUTURE:20260810T100000Z",
		"DTSTART:20260810T140000Z",
		"DTEND:20260810T143000Z",
	), event(
		"UID:series",
		"SUMMARY:Standup (one-off move)",
		"RECURRENCE-ID:20260824T140000Z",
		"DTSTART:20260824T160000Z",
		"DTEND:20260824T163000Z",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")

	want := []string{
		"2026-08-03T10:00:00Z",
		"2026-08-10T14:00:00Z",
		"2026-08-17T14:00:00Z",
		"2026-08-24T16:00:00Z",
	}
	if got := starts(events); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
	if events[3].Summary != "Standup (one-off move)" {
		t.Errorf("the exact-slot override did not win: %q", events[3].Summary)
	}
}

// Two successive "this and all following" edits: the later one takes over from
// its own slot, and the earlier governs only the span between them.
func TestLaterThisAndFutureSupersedesEarlier(t *testing.T) {
	data := wrap(event(
		"UID:series",
		"SUMMARY:Standup",
		"DTSTART:20260803T100000Z",
		"DTEND:20260803T103000Z",
		"RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=4",
	), event(
		"UID:series",
		"SUMMARY:Standup (moved once)",
		"RECURRENCE-ID;RANGE=THISANDFUTURE:20260810T100000Z",
		"DTSTART:20260810T120000Z",
		"DTEND:20260810T123000Z",
	), event(
		"UID:series",
		"SUMMARY:Standup (moved again)",
		"RECURRENCE-ID;RANGE=THISANDFUTURE:20260824T100000Z",
		"DTSTART:20260824T160000Z",
		"DTEND:20260824T163000Z",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")

	want := []struct{ start, summary string }{
		{"2026-08-03T10:00:00Z", "Standup"},
		{"2026-08-10T12:00:00Z", "Standup (moved once)"},
		{"2026-08-17T12:00:00Z", "Standup (moved once)"},
		{"2026-08-24T16:00:00Z", "Standup (moved again)"},
	}
	if len(events) != len(want) {
		t.Fatalf("occurrences = %v, want %d", starts(events), len(want))
	}
	for i, w := range want {
		if got := events[i].Start.UTC().Format(time.RFC3339); got != w.start {
			t.Errorf("occurrence %d starts %s, want %s", i, got, w.start)
		}
		if events[i].Summary != w.summary {
			t.Errorf("occurrence %d is %q, want %q", i, events[i].Summary, w.summary)
		}
	}
}

// A THISANDFUTURE edit whose series is not in the feed has nothing to govern,
// and is simply the event it describes — emitted once, not lost.
func TestOrphanThisAndFutureIsStillAnEvent(t *testing.T) {
	data := wrap(event(
		"UID:gone",
		"SUMMARY:Rescheduled",
		"RECURRENCE-ID;RANGE=THISANDFUTURE:20260817T100000Z",
		"DTSTART:20260817T140000Z",
		"DTEND:20260817T143000Z",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")
	if got := starts(events); len(got) != 1 || got[0] != "2026-08-17T14:00:00Z" {
		t.Errorf("occurrences = %v, want the orphan once", got)
	}
}

// An edit that changes only the details leaves the times alone.
func TestThisAndFutureWithoutAMoveKeepsTimes(t *testing.T) {
	data := wrap(event(
		"UID:series",
		"SUMMARY:Standup",
		"LOCATION:Room 1",
		"DTSTART:20260803T100000Z",
		"DTEND:20260803T103000Z",
		"RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=3",
	), event(
		"UID:series",
		"SUMMARY:Standup",
		"LOCATION:Room 2",
		"RECURRENCE-ID;RANGE=THISANDFUTURE:20260810T100000Z",
		"DTSTART:20260810T100000Z",
		"DTEND:20260810T103000Z",
	))
	events := mustParse(t, data, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z")

	want := []string{"2026-08-03T10:00:00Z", "2026-08-10T10:00:00Z", "2026-08-17T10:00:00Z"}
	if got := starts(events); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("occurrences = %v, want %v", got, want)
	}
	for i, wantRoom := range []string{"Room 1", "Room 2", "Room 2"} {
		if events[i].Location != wantRoom {
			t.Errorf("occurrence %d is in %q, want %q", i, events[i].Location, wantRoom)
		}
	}
}
