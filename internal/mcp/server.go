package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/robertlestak/remote-ical-mcp/internal/ical"
	log "github.com/sirupsen/logrus"
)

type Server struct {
	server    *mcp.Server
	calendars []ical.RemoteCalendar
	byName    map[string]ical.RemoteCalendar
}

func (s *Server) resolveCalendar(calendar string) (string, error) {
	if calendar == "" {
		return "", fmt.Errorf("calendar is required: pass a configured calendar name or an http/https ICS URL")
	}

	// Check if it's a configured calendar name.
	if cal, ok := s.byName[calendar]; ok {
		return cal.URL, nil
	}

	// Validate as URL.
	u, err := url.Parse(calendar)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("unknown calendar %q: must be a configured calendar name (%s) or a valid http/https URL",
			calendar, strings.Join(s.calendarNames(), ", "))
	}

	return calendar, nil
}

func (s *Server) calendarNames() []string {
	names := make([]string, 0, len(s.calendars))
	for _, c := range s.calendars {
		names = append(names, c.Name)
	}
	return names
}

func NewServer(calendars []ical.RemoteCalendar) (*Server, error) {
	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "remote_ical",
		Version: "1.0.0",
	}, nil)

	byName := make(map[string]ical.RemoteCalendar, len(calendars))
	for _, c := range calendars {
		byName[c.Name] = c
	}

	s := &Server{
		server:    server,
		calendars: calendars,
		byName:    byName,
	}

	// Register tools
	s.registerTools()

	return s, nil
}

func (s *Server) StartHTTP(ctx context.Context, port string) error {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.server
	}, nil)

	log.WithField("port", port).Info("starting MCP HTTP server")
	return http.ListenAndServe(":"+port, handler)
}

func (s *Server) Start(ctx context.Context) error {
	transport := &mcp.StdioTransport{}
	return s.server.Run(ctx, transport)
}

// toolError renders an error as a tool result rather than a protocol error, so
// the model sees the message and can correct its call.
func toolError(err error) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
		IsError: true,
	}, nil, nil
}

func toolJSON(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolError(err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// parseWindow converts the optional RFC3339 date arguments into a time range.
func parseWindow(startRaw, endRaw string) (*time.Time, *time.Time, error) {
	var start, end *time.Time
	if startRaw != "" {
		t, err := time.Parse(time.RFC3339, startRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start_date %q: expected RFC3339 (e.g. 2026-01-01T00:00:00Z): %w", startRaw, err)
		}
		start = &t
	}
	if endRaw != "" {
		t, err := time.Parse(time.RFC3339, endRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end_date %q: expected RFC3339 (e.g. 2026-12-31T23:59:59Z): %w", endRaw, err)
		}
		end = &t
	}
	if start != nil && end != nil && end.Before(*start) {
		return nil, nil, fmt.Errorf("end_date %s is before start_date %s", endRaw, startRaw)
	}
	return start, end, nil
}

func (s *Server) registerTools() {
	// List available calendars
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_calendars",
		Description: "List all configured remote calendars, with their names and descriptions",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		return toolJSON(s.calendars)
	})

	type listEventsArgs struct {
		Calendar  string `json:"calendar" jsonschema:"Name of a configured calendar or a direct ICS URL"`
		StartDate string `json:"start_date,omitempty" jsonschema:"Optional. Only return events overlapping on or after this time (RFC3339, e.g. 2026-01-01T00:00:00Z). Defaults to one year ago."`
		EndDate   string `json:"end_date,omitempty" jsonschema:"Optional. Only return events overlapping before this time (RFC3339, e.g. 2026-12-31T23:59:59Z). Defaults to two years ahead."`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_events",
		Description: "Fetch events from a remote ICS calendar. Recurring events are expanded into individual occurrences within the requested date range, and events overlapping the range boundaries are included.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listEventsArgs) (*mcp.CallToolResult, any, error) {
		calURL, err := s.resolveCalendar(args.Calendar)
		if err != nil {
			return toolError(err)
		}

		startDate, endDate, err := parseWindow(args.StartDate, args.EndDate)
		if err != nil {
			return toolError(err)
		}

		events, err := ical.FetchAndParse(calURL, startDate, endDate)
		if err != nil {
			return toolError(err)
		}

		return toolJSON(events)
	})

	type searchEventsArgs struct {
		Calendar  string `json:"calendar" jsonschema:"Name of a configured calendar or a direct ICS URL"`
		Query     string `json:"query" jsonschema:"Search term to filter events by summary, description, or location"`
		StartDate string `json:"start_date,omitempty" jsonschema:"Optional. Only search events overlapping on or after this time (RFC3339). Defaults to one year ago."`
		EndDate   string `json:"end_date,omitempty" jsonschema:"Optional. Only search events overlapping before this time (RFC3339). Defaults to two years ahead."`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search_events",
		Description: "Search for events matching a query string in summary, description, or location. Recurring events are expanded into individual occurrences before matching.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchEventsArgs) (*mcp.CallToolResult, any, error) {
		calURL, err := s.resolveCalendar(args.Calendar)
		if err != nil {
			return toolError(err)
		}

		startDate, endDate, err := parseWindow(args.StartDate, args.EndDate)
		if err != nil {
			return toolError(err)
		}

		events, err := ical.FetchAndParse(calURL, startDate, endDate)
		if err != nil {
			return toolError(err)
		}

		// Filter by search query
		filtered := []ical.Event{}
		query := strings.ToLower(args.Query)
		for _, e := range events {
			if strings.Contains(strings.ToLower(e.Summary), query) ||
				strings.Contains(strings.ToLower(e.Description), query) ||
				strings.Contains(strings.ToLower(e.Location), query) {
				filtered = append(filtered, e)
			}
		}

		return toolJSON(filtered)
	})
}
