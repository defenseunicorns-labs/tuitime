# tuitime

A small terminal UI for filling out a ClickTime timesheet. It is written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea), Bubbles, and Lip Gloss.

## Features

- View projects, tasks, time off, daily hours, and totals in a Monday–Sunday table
- Mark today with `*`, ClickTime timesheet period ends with `+`, and both with `※`
- Show each timesheet period's current status and highlight the period containing the selected day
- Center the application within the terminal viewport
- Move between days and weeks, and jump back to today
- Create project time by choosing a project and task
- Create time off by choosing an available leave type
- Enter the date, hours, and notes
- Review an entry before it is sent to ClickTime
- Edit an existing entry and confirm the update
- Submit the ClickTime timesheet containing the selected day for approval
- Filter client, project, and task pickers

ClickTime calls projects **Jobs** in its API; tuitime uses the friendlier “Project” label in the UI.

## Requirements

- Go 1.24 or newer
- A ClickTime account with REST API v2 access
- A ClickTime authentication token
- A terminal at least 100 columns wide for the weekly table

Get your token in ClickTime under **My Preferences → Authentication Token**. The token is read only from `CLICKTIME_TOKEN` and is never displayed.

## Run

```sh
export CLICKTIME_TOKEN='your-clicktime-api-token'
go run ./cmd/tuitime
```

Or build a binary:

```sh
go build -o tuitime ./cmd/tuitime
./tuitime
```

Running without `CLICKTIME_TOKEN` exits with an error before the TUI starts.

## Keys

### Weekly view

| Key | Action |
| --- | --- |
| `kj↑↓` | Select a project/task row |
| `hl←→` | Select a day |
| `[]` | Previous or next week |
| `n` | Add an entry in the selected day |
| `e` `enter` | Edit the selected cell; quick-add time if it is empty |
| `s` | Review and submit the timesheet containing the selected day |
| `t` | Current week |
| `r` | Refresh |
| `q` | Quit |

### Entry workflow

- Adding time first asks whether it is **Projects** or **Time Off**.
- Pressing `e` on an empty project/task cell opens a new entry for that date and row.
- In a picker, `/` starts filtering, `enter` selects, `esc` goes back one page, and `q` cancels the entry flow.
- The selected date is read-only in the entry form.
- In the form, `tab` moves between fields, `ctrl+r` opens review, and `esc` returns to the previous picker.
- On review, `y` `enter` submits; `b` `esc` goes back.
- `ctrl+c` exits from any screen.

### Timesheet submission

- Press `s` to load the ClickTime timesheet containing the selected day.
- The weekly view identifies each visible period as open, submitted, approved, rejected, or an API-provided status; the selected day's period is highlighted.
- The confirmation screen shows ClickTime's full period, status, and total hours.
- Submission is offered only when ClickTime reports `Submit` as an available action.
- Press `y` or `enter` to attest that the timesheet is accurate and submit the entire period for approval; `b` or `esc` cancels.

## Development

```sh
go test ./...
go vet ./...
```

The API client is in [`internal/clicktime`](internal/clicktime), and the Bubble Tea application is in [`internal/tui`](internal/tui).
