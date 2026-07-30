# tuitime

A small terminal UI for filling out a ClickTime timesheet. It is written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea), Bubbles, and Lip Gloss.

## Features

- View projects, tasks, time off, daily hours, and totals in a Monday–Sunday table
- Center the application within the terminal viewport
- Move between days and weeks, and jump back to today
- Create project time by choosing a project and task
- Create time off by choosing an available leave type
- Enter the date, hours, and notes
- Review an entry before it is sent to ClickTime
- Edit an existing entry and confirm the update
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
| `k`, `j` | Select a project/task row |
| `h`, `l` | Select a day |
| `←`, `→` | Previous or next week |
| `n` | Add an entry in the selected day |
| `e` or `enter` | Edit the selected cell; quick-add time if it is empty |
| `t` | Current week |
| `r` | Refresh |
| `q` | Quit |

### Entry workflow

- Adding time first asks whether it is **Projects** or **Time Off**.
- Pressing `e` on an empty project/task cell opens a new entry for that date and row.
- In a picker, `/` starts filtering, `enter` selects, `esc` goes back one page, and `q` cancels the entry flow.
- The selected date is read-only in the entry form.
- In the form, `tab` moves between fields, `ctrl+r` opens review, and `esc` returns to the previous picker.
- On review, `y` or `enter` submits; `b` or `esc` goes back.
- `ctrl+c` exits from any screen.

## Development

```sh
go test ./...
go vet ./...
```

The API client is in [`internal/clicktime`](internal/clicktime), and the Bubble Tea application is in [`internal/tui`](internal/tui).
