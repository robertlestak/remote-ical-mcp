# Remote iCal MCP Server

An MCP (Model Context Protocol) server that exposes remote ICS (iCalendar) calendar data through a standardized interface.

## Features

- Fetch and parse remote ICS calendar files
- Support for multiple configured calendars or ad-hoc URLs
- Date range filtering
- Text search across events
- Full event details including:
  - Recurring events (RRULE)
  - Attendees and organizer
  - Categories/tags
  - All-day event detection
  - Location, status, and descriptions

## Installation

```bash
go build -o remote-ical-mcp ./cmd/server/server.go
```

## Configuration

Create a `calendars.yaml` or `calendars.json` file:

**YAML:**
```yaml
calendars:
  - name: work
    url: https://example.com/work-calendar.ics
  - name: personal
    url: https://example.com/personal-calendar.ics
```

**JSON:**
```json
{
  "calendars": [
    {"name": "work", "url": "https://example.com/work-calendar.ics"},
    {"name": "personal", "url": "https://example.com/personal-calendar.ics"}
  ]
}
```

## Usage

### Stdio Transport (for MCP clients)
```bash
./remote-ical-mcp -transport stdio -config calendars.yaml
```

### HTTP Transport (for testing)
```bash
./remote-ical-mcp -transport http -port 8080 -config calendars.yaml
```

## Available Tools

### `list_calendars`
List all configured calendars.

**Parameters:** None

### `list_events`
Fetch and list events from a calendar.

**Parameters:**
- `calendar` (required): Calendar name or direct ICS URL
- `start_date` (optional): Filter events from this date (RFC3339 format)
- `end_date` (optional): Filter events until this date (RFC3339 format)

**Example:**
```json
{
  "calendar": "work",
  "start_date": "2025-01-01T00:00:00Z",
  "end_date": "2025-12-31T23:59:59Z"
}
```

### `search_events`
Search for events matching a query string.

**Parameters:**
- `calendar` (required): Calendar name or direct ICS URL
- `query` (required): Search term for summary, description, or location
- `start_date` (optional): Filter events from this date (RFC3339 format)
- `end_date` (optional): Filter events until this date (RFC3339 format)

**Example:**
```json
{
  "calendar": "work",
  "query": "meeting",
  "start_date": "2025-11-01T00:00:00Z"
}
```

## Event Data Structure

```json
{
  "uid": "event-unique-id",
  "summary": "Event Title",
  "description": "Event description",
  "location": "Meeting Room A",
  "start": "2025-11-26T10:00:00Z",
  "end": "2025-11-26T11:00:00Z",
  "status": "CONFIRMED",
  "organizer": "mailto:organizer@example.com",
  "attendees": ["mailto:attendee1@example.com", "mailto:attendee2@example.com"],
  "categories": ["Work", "Important"],
  "recurrence_rule": "FREQ=WEEKLY;BYDAY=MO,WE,FR",
  "all_day": false
}
```

## Environment Variables

- `LOG_LEVEL`: Set logging level (debug, info, warn, error). Default: info

## License

MIT
