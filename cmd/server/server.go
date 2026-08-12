package main

import (
	"context"
	"flag"
	"os"

	"github.com/robertlestak/remote-ical-mcp/internal/config"
	"github.com/robertlestak/remote-ical-mcp/internal/ical"
	"github.com/robertlestak/remote-ical-mcp/internal/mcp"
	log "github.com/sirupsen/logrus"
)

func init() {
	ll, err := log.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		ll = log.InfoLevel
	}
	log.SetLevel(ll)
}

func main() {
	l := log.WithField("fn", "main")
	mcpTransport := flag.String("transport", "http", "MCP transport type: stdio or http")
	mcpPort := flag.String("port", "8080", "MCP HTTP server port")
	configFile := flag.String("config", "calendars.yaml", "Path to calendars config file (yaml or json)")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		l.WithError(err).Fatal("failed to load config")
	}

	calendars := make([]ical.RemoteCalendar, 0, len(cfg.Calendars))
	for _, cal := range cfg.Calendars {
		calendars = append(calendars, ical.RemoteCalendar{
			Name:        cal.Name,
			Description: cal.Description,
			URL:         cal.URL,
		})
	}

	server, err := mcp.NewServer(calendars)
	if err != nil {
		l.WithError(err).Fatal("failed to create MCP server")
	}

	if *mcpTransport == "stdio" {
		l.Info("starting MCP server on stdio transport")
		if err := server.Start(context.Background()); err != nil {
			l.WithError(err).Fatal("MCP server failed")
		}
	} else {
		l.WithField("port", *mcpPort).Info("starting MCP HTTP server")
		if err := server.StartHTTP(context.Background(), *mcpPort); err != nil {
			l.WithError(err).Fatal("MCP HTTP server failed")
		}
	}
}
