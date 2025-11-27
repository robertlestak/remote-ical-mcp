package ical

import (
	"bytes"
	"io"
	"net/http"
	"time"

	ics "github.com/arran4/golang-ical"
)

type RemoteCalendar struct {
	URL         string `json:"url" yaml:"url"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

type Event struct {
	UID         string    `json:"uid"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Status      string    `json:"status"`
	Organizer   string    `json:"organizer,omitempty"`
	Attendees   []string  `json:"attendees,omitempty"`
	Categories  []string  `json:"categories,omitempty"`
	RecurRule   string    `json:"recurrence_rule,omitempty"`
	AllDay      bool      `json:"all_day"`
}

func FetchAndParse(url string, startDate, endDate *time.Time) ([]Event, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	cal, err := ics.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, event := range cal.Events() {
		e := Event{
			UID: event.Id(),
		}
		if prop := event.GetProperty(ics.ComponentPropertySummary); prop != nil {
			e.Summary = prop.Value
		}
		if prop := event.GetProperty(ics.ComponentPropertyLocation); prop != nil {
			e.Location = prop.Value
		}
		if prop := event.GetProperty(ics.ComponentPropertyStatus); prop != nil {
			e.Status = prop.Value
		}
		if prop := event.GetProperty(ics.ComponentPropertyDescription); prop != nil {
			e.Description = prop.Value
		}
		if prop := event.GetProperty(ics.ComponentPropertyOrganizer); prop != nil {
			e.Organizer = prop.Value
		}
		if prop := event.GetProperty(ics.ComponentPropertyRrule); prop != nil {
			e.RecurRule = prop.Value
		}

		// Get attendees
		for _, prop := range event.Properties {
			if prop.IANAToken == string(ics.ComponentPropertyAttendee) {
				e.Attendees = append(e.Attendees, prop.Value)
			}
			if prop.IANAToken == string(ics.ComponentPropertyCategories) {
				e.Categories = append(e.Categories, prop.Value)
			}
		}

		if start, err := event.GetStartAt(); err == nil {
			e.Start = start
			// Check if all-day event (no time component)
			if prop := event.GetProperty(ics.ComponentPropertyDtStart); prop != nil {
				if len(prop.Value) == 8 { // YYYYMMDD format = all-day
					e.AllDay = true
				}
			}
		}
		if end, err := event.GetEndAt(); err == nil {
			e.End = end
		}

		// Filter by date range
		if startDate != nil && e.Start.Before(*startDate) {
			continue
		}
		if endDate != nil && e.Start.After(*endDate) {
			continue
		}

		events = append(events, e)
	}

	return events, nil
}
