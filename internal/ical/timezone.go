package ical

import (
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

// windowsToIANA maps Microsoft Windows timezone identifiers to IANA zone names.
// Exchange and Outlook emit TZID values like "Pacific Standard Time", which
// time.LoadLocation cannot resolve. Derived from the CLDR windowsZones mapping.
var windowsToIANA = map[string]string{
	"Afghanistan Standard Time":       "Asia/Kabul",
	"Alaskan Standard Time":           "America/Anchorage",
	"Aleutian Standard Time":          "America/Adak",
	"Altai Standard Time":             "Asia/Barnaul",
	"Arab Standard Time":              "Asia/Riyadh",
	"Arabian Standard Time":           "Asia/Dubai",
	"Arabic Standard Time":            "Asia/Baghdad",
	"Argentina Standard Time":         "America/Argentina/Buenos_Aires",
	"Astrakhan Standard Time":         "Europe/Astrakhan",
	"Atlantic Standard Time":          "America/Halifax",
	"AUS Central Standard Time":       "Australia/Darwin",
	"Aus Central W. Standard Time":    "Australia/Eucla",
	"AUS Eastern Standard Time":       "Australia/Sydney",
	"Azerbaijan Standard Time":        "Asia/Baku",
	"Azores Standard Time":            "Atlantic/Azores",
	"Bahia Standard Time":             "America/Bahia",
	"Bangladesh Standard Time":        "Asia/Dhaka",
	"Belarus Standard Time":           "Europe/Minsk",
	"Bougainville Standard Time":      "Pacific/Bougainville",
	"Canada Central Standard Time":    "America/Regina",
	"Cape Verde Standard Time":        "Atlantic/Cape_Verde",
	"Caucasus Standard Time":          "Asia/Yerevan",
	"Cen. Australia Standard Time":    "Australia/Adelaide",
	"Central America Standard Time":   "America/Guatemala",
	"Central Asia Standard Time":      "Asia/Almaty",
	"Central Brazilian Standard Time": "America/Cuiaba",
	"Central Europe Standard Time":    "Europe/Budapest",
	"Central European Standard Time":  "Europe/Warsaw",
	"Central Pacific Standard Time":   "Pacific/Guadalcanal",
	"Central Standard Time":           "America/Chicago",
	"Central Standard Time (Mexico)":  "America/Mexico_City",
	"Chatham Islands Standard Time":   "Pacific/Chatham",
	"China Standard Time":             "Asia/Shanghai",
	"Cuba Standard Time":              "America/Havana",
	"Dateline Standard Time":          "Etc/GMT+12",
	"E. Africa Standard Time":         "Africa/Nairobi",
	"E. Australia Standard Time":      "Australia/Brisbane",
	"E. Europe Standard Time":         "Europe/Chisinau",
	"E. South America Standard Time":  "America/Sao_Paulo",
	"Easter Island Standard Time":     "Pacific/Easter",
	"Eastern Standard Time":           "America/New_York",
	"Eastern Standard Time (Mexico)":  "America/Cancun",
	"Egypt Standard Time":             "Africa/Cairo",
	"Ekaterinburg Standard Time":      "Asia/Yekaterinburg",
	"Fiji Standard Time":              "Pacific/Fiji",
	"FLE Standard Time":               "Europe/Kiev",
	"Georgian Standard Time":          "Asia/Tbilisi",
	"GMT Standard Time":               "Europe/London",
	"Greenland Standard Time":         "America/Godthab",
	"Greenwich Standard Time":         "Atlantic/Reykjavik",
	"GTB Standard Time":               "Europe/Bucharest",
	"Haiti Standard Time":             "America/Port-au-Prince",
	"Hawaiian Standard Time":          "Pacific/Honolulu",
	"India Standard Time":             "Asia/Calcutta",
	"Iran Standard Time":              "Asia/Tehran",
	"Israel Standard Time":            "Asia/Jerusalem",
	"Jordan Standard Time":            "Asia/Amman",
	"Kaliningrad Standard Time":       "Europe/Kaliningrad",
	"Korea Standard Time":             "Asia/Seoul",
	"Libya Standard Time":             "Africa/Tripoli",
	"Line Islands Standard Time":      "Pacific/Kiritimati",
	"Lord Howe Standard Time":         "Australia/Lord_Howe",
	"Magadan Standard Time":           "Asia/Magadan",
	"Magallanes Standard Time":        "America/Punta_Arenas",
	"Marquesas Standard Time":         "Pacific/Marquesas",
	"Mauritius Standard Time":         "Indian/Mauritius",
	"Middle East Standard Time":       "Asia/Beirut",
	"Montevideo Standard Time":        "America/Montevideo",
	"Morocco Standard Time":           "Africa/Casablanca",
	"Mountain Standard Time":          "America/Denver",
	"Mountain Standard Time (Mexico)": "America/Mazatlan",
	"Myanmar Standard Time":           "Asia/Rangoon",
	"N. Central Asia Standard Time":   "Asia/Novosibirsk",
	"Namibia Standard Time":           "Africa/Windhoek",
	"Nepal Standard Time":             "Asia/Katmandu",
	"New Zealand Standard Time":       "Pacific/Auckland",
	"Newfoundland Standard Time":      "America/St_Johns",
	"Norfolk Standard Time":           "Pacific/Norfolk",
	"North Asia East Standard Time":   "Asia/Irkutsk",
	"North Asia Standard Time":        "Asia/Krasnoyarsk",
	"North Korea Standard Time":       "Asia/Pyongyang",
	"Omsk Standard Time":              "Asia/Omsk",
	"Pacific SA Standard Time":        "America/Santiago",
	"Pacific Standard Time":           "America/Los_Angeles",
	"Pacific Standard Time (Mexico)":  "America/Tijuana",
	"Pakistan Standard Time":          "Asia/Karachi",
	"Paraguay Standard Time":          "America/Asuncion",
	"Qyzylorda Standard Time":         "Asia/Qyzylorda",
	"Romance Standard Time":           "Europe/Paris",
	"Russia Time Zone 3":              "Europe/Samara",
	"Russia Time Zone 10":             "Asia/Srednekolymsk",
	"Russia Time Zone 11":             "Asia/Kamchatka",
	"Russian Standard Time":           "Europe/Moscow",
	"SA Eastern Standard Time":        "America/Cayenne",
	"SA Pacific Standard Time":        "America/Bogota",
	"SA Western Standard Time":        "America/La_Paz",
	"Saint Pierre Standard Time":      "America/Miquelon",
	"Sakhalin Standard Time":          "Asia/Sakhalin",
	"Samoa Standard Time":             "Pacific/Apia",
	"Sao Tome Standard Time":          "Africa/Sao_Tome",
	"Saratov Standard Time":           "Europe/Saratov",
	"SE Asia Standard Time":           "Asia/Bangkok",
	"Singapore Standard Time":         "Asia/Singapore",
	"South Africa Standard Time":      "Africa/Johannesburg",
	"South Sudan Standard Time":       "Africa/Juba",
	"Sri Lanka Standard Time":         "Asia/Colombo",
	"Sudan Standard Time":             "Africa/Khartoum",
	"Syria Standard Time":             "Asia/Damascus",
	"Taipei Standard Time":            "Asia/Taipei",
	"Tasmania Standard Time":          "Australia/Hobart",
	"Tocantins Standard Time":         "America/Araguaina",
	"Tokyo Standard Time":             "Asia/Tokyo",
	"Tomsk Standard Time":             "Asia/Tomsk",
	"Tonga Standard Time":             "Pacific/Tongatapu",
	"Transbaikal Standard Time":       "Asia/Chita",
	"Turkey Standard Time":            "Europe/Istanbul",
	"Turks And Caicos Standard Time":  "America/Grand_Turk",
	"Ulaanbaatar Standard Time":       "Asia/Ulaanbaatar",
	"US Eastern Standard Time":        "America/Indianapolis",
	"US Mountain Standard Time":       "America/Phoenix",
	"UTC":                             "Etc/UTC",
	"UTC+12":                          "Etc/GMT-12",
	"UTC+13":                          "Etc/GMT-13",
	"UTC-02":                          "Etc/GMT+2",
	"UTC-08":                          "Etc/GMT+8",
	"UTC-09":                          "Etc/GMT+9",
	"UTC-11":                          "Etc/GMT+11",
	"Venezuela Standard Time":         "America/Caracas",
	"Vladivostok Standard Time":       "Asia/Vladivostok",
	"Volgograd Standard Time":         "Europe/Volgograd",
	"W. Australia Standard Time":      "Australia/Perth",
	"W. Central Africa Standard Time": "Africa/Lagos",
	"W. Europe Standard Time":         "Europe/Berlin",
	"W. Mongolia Standard Time":       "Asia/Hovd",
	"West Asia Standard Time":         "Asia/Tashkent",
	"West Bank Standard Time":         "Asia/Hebron",
	"West Pacific Standard Time":      "Pacific/Port_Moresby",
	"Yakutsk Standard Time":           "Asia/Yakutsk",
	"Yukon Standard Time":             "America/Whitehorse",
}

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
}

func newTZResolver(cal *ics.Calendar) *tzResolver {
	r := &tzResolver{
		cache:      make(map[string]*time.Location),
		vtimezones: make(map[string]*vtimezone),
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
	anchor := time.Date(wall.Year()-1, time.January, 1, h, m, s, 0, time.UTC)

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

// load resolves a TZID to a time.Location, trying IANA names first, then the
// Windows mapping, then any VTIMEZONE definition embedded in the feed. It
// returns nil when the zone cannot be determined.
//
// The wall argument is the local time being interpreted; it is only used to pick
// the correct offset from a VTIMEZONE fallback, which has no single fixed offset.
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
	if iana, ok := windowsToIANA[tzid]; ok {
		if loc, err := time.LoadLocation(iana); err == nil {
			r.cache[tzid] = loc
			return loc
		}
	}
	// Some feeds prefix TZIDs, e.g. "/mozilla.org/20050126_1/Europe/Berlin".
	if idx := strings.LastIndex(tzid, "/"); idx >= 0 && idx < len(tzid)-1 {
		if loc, err := time.LoadLocation(tzid[idx+1:]); err == nil {
			r.cache[tzid] = loc
			return loc
		}
	}

	// Fall back to the offsets declared by the feed's own VTIMEZONE component.
	// These are not cached: the offset is date-dependent because of DST.
	if vtz, ok := r.vtimezones[tzid]; ok {
		return time.FixedZone(tzid, vtz.offsetAt(wall))
	}
	return nil
}
