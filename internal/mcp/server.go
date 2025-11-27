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
	calendars map[string]string // name -> URL
}

func (s *Server) resolveCalendar(calendar string) (string, error) {
	// Check if it's a configured calendar name
	if configURL, ok := s.calendars[calendar]; ok {
		return configURL, nil
	}
	
	// Validate as URL
	u, err := url.Parse(calendar)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("calendar must be a configured calendar name or valid http/https URL")
	}
	
	return calendar, nil
}

func NewServer(calendars map[string]string) (*Server, error) {
	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "remote_ical",
		Version: "1.0.0",
	}, nil)

	s := &Server{
		server:    server,
		calendars: calendars,
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

func (s *Server) registerTools() {
	// List available calendars
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_calendars",
		Description: "List all configured remote calendars",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		data, err := json.MarshalIndent(s.calendars, "", "  ")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	type listEventsArgs struct {
		Calendar  string `json:"calendar" jsonschema:"Name of a configured calendar or a direct ICS URL"`
		StartDate string `json:"start_date" jsonschema:"Filter events starting from this date (RFC3339 format, e.g. 2025-01-01T00:00:00Z)"`
		EndDate   string `json:"end_date" jsonschema:"Filter events up to this date (RFC3339 format, e.g. 2025-12-31T23:59:59Z)"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_events",
		Description: "Fetch and list all events from a remote ICS calendar",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listEventsArgs) (*mcp.CallToolResult, any, error) {
		url, err := s.resolveCalendar(args.Calendar)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		var startDate, endDate *time.Time
		if args.StartDate != "" {
			t, err := time.Parse(time.RFC3339, args.StartDate)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "invalid start_date: " + err.Error()}},
					IsError: true,
				}, nil, nil
			}
			startDate = &t
		}
		if args.EndDate != "" {
			t, err := time.Parse(time.RFC3339, args.EndDate)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "invalid end_date: " + err.Error()}},
					IsError: true,
				}, nil, nil
			}
			endDate = &t
		}

		events, err := ical.FetchAndParse(url, startDate, endDate)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		data, err := json.MarshalIndent(events, "", "  ")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	type searchEventsArgs struct {
		Calendar  string `json:"calendar" jsonschema:"Name of a configured calendar or a direct ICS URL"`
		Query     string `json:"query" jsonschema:"Search term to filter events by summary, description, or location"`
		StartDate string `json:"start_date" jsonschema:"Filter events starting from this date (RFC3339 format)"`
		EndDate   string `json:"end_date" jsonschema:"Filter events up to this date (RFC3339 format)"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search_events",
		Description: "Search for events matching a query string in summary, description, or location",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchEventsArgs) (*mcp.CallToolResult, any, error) {
		url, err := s.resolveCalendar(args.Calendar)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		var startDate, endDate *time.Time
		if args.StartDate != "" {
			t, err := time.Parse(time.RFC3339, args.StartDate)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "invalid start_date: " + err.Error()}},
					IsError: true,
				}, nil, nil
			}
			startDate = &t
		}
		if args.EndDate != "" {
			t, err := time.Parse(time.RFC3339, args.EndDate)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "invalid end_date: " + err.Error()}},
					IsError: true,
				}, nil, nil
			}
			endDate = &t
		}

		events, err := ical.FetchAndParse(url, startDate, endDate)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		// Filter by search query
		var filtered []ical.Event
		query := strings.ToLower(args.Query)
		for _, e := range events {
			if strings.Contains(strings.ToLower(e.Summary), query) ||
				strings.Contains(strings.ToLower(e.Description), query) ||
				strings.Contains(strings.ToLower(e.Location), query) {
				filtered = append(filtered, e)
			}
		}

		data, err := json.MarshalIndent(filtered, "", "  ")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}
