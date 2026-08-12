package ical

import (
	"sort"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

// tzTransition is one STANDARD or DAYLIGHT rule inside a VTIMEZONE component.
type tzTransition struct {
	offset     int // seconds east of UTC, from TZOFFSETTO
	name       string
	start      time.Time // DTSTART, interpreted as wall-clock in the preceding offset
	opt        *rrule.ROption
	isDaylight bool
}

// vtimezone is the fallback interpretation of a VTIMEZONE component, used when
// a TZID matches neither an IANA name nor a known Windows name.
type vtimezone struct {
	transitions []tzTransition
}

// tzResolver maps TZID strings to time.Locations for a single calendar. Lookups
// are cached because a feed typically repeats a handful of TZIDs across hundreds
// of events.
type tzResolver struct {
	cache      map[string]*time.Location
	vtimezones map[string]*vtimezone
	unresolved map[string]bool
}

func newTZResolver(cal *ics.Calendar) *tzResolver {
	r := &tzResolver{
		cache:      make(map[string]*time.Location),
		vtimezones: make(map[string]*vtimezone),
		unresolved: make(map[string]bool),
	}
	for _, comp := range cal.Components {
		vtz, ok := comp.(*ics.VTimezone)
		if !ok {
			continue
		}
		prop := vtz.GetProperty(ics.ComponentProperty(ics.PropertyTzid))
		if prop == nil || prop.Value == "" {
			continue
		}
		if parsed := parseVTimezone(vtz); parsed != nil {
			r.vtimezones[prop.Value] = parsed
		}
	}
	return r
}

func parseVTimezone(vtz *ics.VTimezone) *vtimezone {
	out := &vtimezone{}
	for _, sub := range vtz.Components {
		var base *ics.ComponentBase
		daylight := false
		switch c := sub.(type) {
		case *ics.Standard:
			base = &c.ComponentBase
		case *ics.Daylight:
			base = &c.ComponentBase
			daylight = true
		default:
			continue
		}

		offProp := base.GetProperty(ics.ComponentProperty(ics.PropertyTzoffsetto))
		if offProp == nil {
			continue
		}
		offset, err := parseUTCOffset(offProp.Value)
		if err != nil {
			continue
		}

		t := tzTransition{offset: offset, isDaylight: daylight}
		if p := base.GetProperty(ics.ComponentProperty(ics.PropertyTzname)); p != nil {
			t.name = p.Value
		}
		if p := base.GetProperty(ics.ComponentPropertyDtStart); p != nil {
			if start, err := time.ParseInLocation("20060102T150405", p.Value, time.UTC); err == nil {
				t.start = start
			}
		}
		// The rule is stored unbuilt: its DTSTART has to be re-anchored per
		// query, see latestOccurrence.
		if p := base.GetProperty(ics.ComponentPropertyRrule); p != nil {
			if ro, err := rrule.StrToROption(p.Value); err == nil {
				t.opt = ro
			}
		}
		out.transitions = append(out.transitions, t)
	}
	if len(out.transitions) == 0 {
		return nil
	}
	return out
}

// parseUTCOffset converts an iCalendar UTC offset ("-0800", "+0530", "+053045")
// into seconds east of UTC.
func parseUTCOffset(s string) (int, error) {
	s = strings.TrimSpace(s)
	sign := 1
	if strings.HasPrefix(s, "-") {
		sign = -1
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if len(s) != 4 && len(s) != 6 {
		return 0, strconv.ErrSyntax
	}
	h, err := strconv.Atoi(s[0:2])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(s[2:4])
	if err != nil {
		return 0, err
	}
	sec := 0
	if len(s) == 6 {
		if sec, err = strconv.Atoi(s[4:6]); err != nil {
			return 0, err
		}
	}
	return sign * (h*3600 + m*60 + sec), nil
}

// latestOccurrence returns the most recent time this transition fires at or
// before wall.
//
// The rule is re-anchored rather than started from its own DTSTART. Microsoft
// emits DTSTART:16010101T000000 in every VTIMEZONE, and rrule-go yields no
// occurrences at all for a DTSTART that far back, which previously made every
// transition invisible and pinned such zones to their standard-time offset
// year round. BYMONTH and BYDAY fully determine the date, so the anchor year is
// immaterial as long as it precedes the query; only the clock time carries over.
func (t tzTransition) latestOccurrence(wall time.Time) (time.Time, bool) {
	if t.opt == nil {
		if t.start.IsZero() || t.start.After(wall) {
			return time.Time{}, false
		}
		return t.start, true
	}

	h, m, s := 0, 0, 0
	if !t.start.IsZero() {
		h, m, s = t.start.Clock()
	}

	// BYMONTH and BYDAY fully determine the date, so January is as good an
	// anchor as any. Without BYMONTH the rule falls back to DTSTART's own month
	// and day to place the transition, and anchoring to January would move it
	// there — pinning the zone to one offset for the rest of the year.
	month, day := time.January, 1
	if len(t.opt.Bymonth) == 0 && !t.start.IsZero() {
		month, day = t.start.Month(), t.start.Day()
	}
	anchor := time.Date(wall.Year()-1, month, day, h, m, s, 0, time.UTC)

	opt := *t.opt
	opt.Dtstart = anchor
	rr, err := rrule.NewRRule(opt)
	if err != nil {
		return time.Time{}, false
	}
	// A full year of lookback guarantees at least one occurrence of an annual
	// rule at or before wall.
	window := rr.Between(anchor, wall, true)
	if len(window) == 0 {
		return time.Time{}, false
	}
	return window[len(window)-1], true
}

// offsetAt returns the UTC offset this VTIMEZONE defines for the given wall-clock
// time, by finding the most recent transition occurring at or before it.
func (v *vtimezone) offsetAt(wall time.Time) int {
	best := v.transitions[0].offset
	var bestTime time.Time
	found := false

	for _, t := range v.transitions {
		occ, ok := t.latestOccurrence(wall)
		if !ok {
			continue
		}
		if !found || occ.After(bestTime) {
			best, bestTime, found = t.offset, occ, true
		}
	}

	if !found {
		// Before any transition: prefer the standard-time offset.
		for _, t := range v.transitions {
			if !t.isDaylight {
				return t.offset
			}
		}
	}
	return best
}

// load resolves a TZID to a time.Location, or nil when it cannot be determined.
//
// There is no Windows-to-IANA name table here, and deliberately so. Exchange
// names its zones "Pacific Standard Time", which time.LoadLocation cannot
// resolve — but it also ships a VTIMEZONE defining every TZID it references, so
// the feed answers the question about itself. A static table is a second,
// staler copy of that answer: it has to be extended for every zone nobody
// thought to enumerate, and its entries rot as the IANA database renames zones.
//
// IANA is still tried first, because a real location carries the full history
// of a zone and knows which local times never existed. The feed's own rules are
// the fallback, which is the case the table used to cover.
//
// The wall argument is the local time being interpreted; it is only used to pick
// the offset from a VTIMEZONE, which has no single fixed one.
func (r *tzResolver) load(tzid string, wall time.Time) *time.Location {
	if tzid == "" {
		return nil
	}
	if loc, ok := r.cache[tzid]; ok && loc != nil {
		return loc
	}

	if loc, err := time.LoadLocation(tzid); err == nil {
		r.cache[tzid] = loc
		return loc
	}
	// Some feeds prefix TZIDs, e.g. "/mozilla.org/20050126_1/Europe/Berlin".
	if idx := strings.LastIndex(tzid, "/"); idx >= 0 && idx < len(tzid)-1 {
		if loc, err := time.LoadLocation(tzid[idx+1:]); err == nil {
			r.cache[tzid] = loc
			return loc
		}
	}

	// The offsets the feed declares for itself. Not cached: the offset is
	// date-dependent because of DST, so it is resolved per instant.
	if vtz, ok := r.vtimezones[tzid]; ok {
		return time.FixedZone(tzid, vtz.offsetAt(wall))
	}

	// Neither IANA nor the feed can place this zone. Recorded rather than
	// passed over, because the caller's fallback to local time produces a
	// plausible-looking wrong answer.
	r.unresolved[tzid] = true
	return nil
}

// Unresolved reports the TZIDs that could not be placed, so a feed that is
// being silently misread is visible as something other than odd-looking times.
func (r *tzResolver) Unresolved() []string {
	out := make([]string, 0, len(r.unresolved))
	for tzid := range r.unresolved {
		out = append(out, tzid)
	}
	sort.Strings(out)
	return out
}

// resolveWall places a wall-clock reading on the timeline in the zone a TZID
// names, falling back to local time when the zone cannot be determined.
//
// Every call resolves the offset afresh for the date being placed, which is
// what keeps a recurring series on its local hour. The alternative — resolving
// once and reusing the location — silently freezes the offset, because the
// VTIMEZONE fallback can only ever return a fixed zone: it answers for the one
// instant it was asked about.
func (r *tzResolver) resolveWall(wall time.Time, tzid string) time.Time {
	loc := r.load(tzid, wall)
	if loc == nil {
		loc = time.Local
	}
	return time.Date(wall.Year(), wall.Month(), wall.Day(),
		wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), loc)
}
