package tui

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"tuitime/internal/clicktime"
)

type screen int

const (
	screenLoading screen = iota
	screenDashboard
	screenPicker
	screenTaskLoading
	screenForm
	screenReview
	screenSaving
	screenDeleteReview
	screenTimesheetLoading
	screenTimesheetReview
	screenTimesheetSubmitting
	screenError
)

type pickerKind int

const (
	pickerCategory pickerKind = iota
	pickerJob
	pickerTask
	pickerTimeOff
	pickerEntry
)

type entryKind int

const (
	projectEntry entryKind = iota
	timeOffEntry
)

type pickerItem struct {
	id          string
	title       string
	description string
}

func (i pickerItem) Title() string       { return i.title }
func (i pickerItem) Description() string { return i.description }
func (i pickerItem) FilterValue() string { return i.title + " " + i.description }

type draft struct {
	kind            entryKind
	date            string
	hours           float64
	comment         string
	clientID        string
	clientName      string
	jobID           string
	jobName         string
	taskID          string
	taskName        string
	timeOffTypeID   string
	timeOffTypeName string
	entryID         string
	returnDashboard bool
}

type trackedEntry struct {
	kind    entryKind
	project clicktime.TimeEntry
	timeOff clicktime.TimeOffEntry
}

func (e trackedEntry) key() string {
	if e.kind == timeOffEntry {
		return "timeoff:" + e.timeOff.Key()
	}
	return "project:" + e.project.Key()
}

func (e trackedEntry) entryID() string {
	if e.kind == timeOffEntry {
		return e.timeOff.Key()
	}
	return e.project.Key()
}

func (e trackedEntry) date() string {
	if e.kind == timeOffEntry {
		return e.timeOff.Date
	}
	return e.project.Date
}

func (e trackedEntry) hours() float64 {
	if e.kind == timeOffEntry {
		return float64(e.timeOff.Hours)
	}
	return float64(e.project.Hours)
}

func (e trackedEntry) comment() string {
	if e.kind == timeOffEntry {
		return e.timeOff.Notes
	}
	return e.project.Comment
}

func totalTrackedHours(entries []trackedEntry) float64 {
	var total float64
	for _, entry := range entries {
		total += entry.hours()
	}
	return total
}

type timesheetRow struct {
	kind    entryKind
	jobID   string
	taskID  string
	project string
	task    string
	entries [7][]trackedEntry
	hours   [7]float64
	total   float64
}

type submissionChargeCode struct {
	key   string
	alias string
	name  string
	total float64
}

type allDataMsg struct {
	me             clicktime.Me
	clients        []clicktime.ClientResource
	jobs           []clicktime.Job
	tasks          []clicktime.Task
	timeOffTypes   []clicktime.TimeOffType
	entries        []clicktime.TimeEntry
	timeOffEntries []clicktime.TimeOffEntry
	timesheets     []clicktime.Timesheet
	week           time.Time
}

type entriesMsg struct {
	entries        []clicktime.TimeEntry
	timeOffEntries []clicktime.TimeOffEntry
	timesheets     []clicktime.Timesheet
	week           time.Time
}

type tasksMsg struct {
	tasks []clicktime.Task
}

type timesheetReadyMsg struct {
	timesheet            clicktime.Timesheet
	actions              []clicktime.TimesheetAction
	attestationStatement string
	entries              []clicktime.TimeEntry
	timeOffEntries       []clicktime.TimeOffEntry
}

type timesheetSubmittedMsg struct {
	timesheet clicktime.Timesheet
}

type savedMsg struct {
	status string
}

type operationErrorMsg struct {
	op  string
	err error
}

// Model is the Bubble Tea application model.
type Model struct {
	api *clicktime.Client
	now func() time.Time

	screen      screen
	width       int
	height      int
	loadingText string
	spinner     spinner.Model

	me             clicktime.Me
	clients        []clicktime.ClientResource
	jobs           []clicktime.Job
	tasks          []clicktime.Task
	timeOffTypes   []clicktime.TimeOffType
	entries        []clicktime.TimeEntry
	timeOffEntries []clicktime.TimeOffEntry
	timesheets     []clicktime.Timesheet

	weekStart            time.Time
	cursor               int
	dayCursor            int
	status               string
	lastError            error
	pendingDeleteEntries []trackedEntry
	timesheetToSubmit    clicktime.Timesheet
	submissionEntries    []clicktime.TimeEntry
	submissionTimeOff    []clicktime.TimeOffEntry
	attestationStatement string

	picker         list.Model
	pickerKind     pickerKind
	draft          draft
	availableTasks []clicktime.Task

	hoursInput textinput.Model
	notesInput textarea.Model
	formFocus  int
}

// New constructs the application and starts it on the current week.
func New(api *clicktime.Client) Model {
	return NewAt(api, time.Now)
}

// NewAt allows tests to supply a deterministic clock.
func NewAt(api *clicktime.Client, now func() time.Time) Model {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(accent)

	hoursInput := textinput.New()
	hoursInput.Prompt = ""
	hoursInput.Placeholder = "e.g. 7.5"
	hoursInput.CharLimit = 8
	hoursInput.Width = 16

	notesInput := textarea.New()
	notesInput.Prompt = ""
	notesInput.Placeholder = "What did you work on?"
	notesInput.ShowLineNumbers = false
	notesInput.SetHeight(5)

	start := startOfWeek(now())
	return Model{
		api:         api,
		now:         now,
		screen:      screenLoading,
		loadingText: "Loading your ClickTime week",
		spinner:     spin,
		weekStart:   start,
		hoursInput:  hoursInput,
		notesInput:  notesInput,
	}
}

func (m Model) Init() tea.Cmd {
	return m.withSpinner(loadAllCmd(m.api, m.weekStart))
}

func (m Model) withSpinner(cmd tea.Cmd) tea.Cmd {
	return tea.Batch(m.spinner.Tick, cmd)
}

func (m Model) spinnerActive() bool {
	switch m.screen {
	case screenLoading, screenTaskLoading, screenSaving, screenTimesheetLoading, screenTimesheetSubmitting:
		return true
	default:
		return false
	}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil
	case spinner.TickMsg:
		if !m.spinnerActive() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case allDataMsg:
		m.me = msg.me
		m.clients = msg.clients
		m.jobs = msg.jobs
		m.tasks = msg.tasks
		m.timeOffTypes = msg.timeOffTypes
		m.entries = sortedEntries(msg.entries)
		m.timeOffEntries = sortedTimeOffEntries(msg.timeOffEntries)
		m.timesheets = append([]clicktime.Timesheet(nil), msg.timesheets...)
		m.weekStart = msg.week
		m.cursor = 0
		m.dayCursor = dayIndexInWeek(m.now(), msg.week)
		m.screen = screenDashboard
		m.lastError = nil
		return m, nil
	case entriesMsg:
		if sameDay(msg.week, m.weekStart) {
			m.entries = sortedEntries(msg.entries)
			m.timeOffEntries = sortedTimeOffEntries(msg.timeOffEntries)
			m.timesheets = append([]clicktime.Timesheet(nil), msg.timesheets...)
			m.cursor = min(m.cursor, max(0, len(m.timesheetRows())-1))
		}
		m.screen = screenDashboard
		m.lastError = nil
		return m, nil
	case tasksMsg:
		resolved := resolveTasks(msg.tasks, m.tasks)
		m.availableTasks = append([]clicktime.Task(nil), resolved...)
		m.tasks = mergeTasks(m.tasks, resolved)
		if len(resolved) == 0 {
			m.screen = screenDashboard
			m.status = "No tasks are available for that project."
			return m, nil
		}
		m.openTaskPicker(resolved)
		return m, nil
	case timesheetReadyMsg:
		m.timesheetToSubmit = msg.timesheet
		m.submissionEntries = sortedEntries(msg.entries)
		m.submissionTimeOff = sortedTimeOffEntries(msg.timeOffEntries)
		m.attestationStatement = strings.TrimSpace(msg.attestationStatement)
		if !hasTimesheetAction(msg.actions, "Submit") {
			status := strings.TrimSpace(msg.timesheet.Status)
			if status == "" {
				status = "unknown"
			}
			m.screen = screenDashboard
			m.lastError = fmt.Errorf("ClickTime does not currently allow this timesheet to be submitted (status: %s)", status)
			return m, nil
		}
		m.screen = screenTimesheetReview
		m.lastError = nil
		return m, nil
	case timesheetSubmittedMsg:
		m.timesheetToSubmit = msg.timesheet
		m.status = "Timesheet submitted for approval."
		m.screen = screenLoading
		m.loadingText = "Refreshing your week"
		m.lastError = nil
		return m, m.withSpinner(loadEntriesCmd(m.api, m.weekStart))
	case savedMsg:
		if msg.status != "" {
			m.status = msg.status
		} else {
			verb := "created"
			if m.draft.entryID != "" {
				verb = "updated"
			}
			entryLabel := "Time entry"
			if m.draft.kind == timeOffEntry {
				entryLabel = "Time off entry"
			}
			m.status = entryLabel + " " + verb + "."
		}
		m.screen = screenLoading
		m.loadingText = "Refreshing your week"
		m.pendingDeleteEntries = nil
		return m, m.withSpinner(loadEntriesCmd(m.api, m.weekStart))
	case operationErrorMsg:
		m.lastError = msg.err
		switch msg.op {
		case "initial load":
			m.screen = screenError
		case "save time entry":
			m.screen = screenReview
		case "delete time entry":
			m.screen = screenDeleteReview
		case "submit timesheet":
			m.screen = screenTimesheetReview
		default:
			m.screen = screenDashboard
		}
		return m, nil
	}

	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m.updateComponent(message)
	}
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.screen {
	case screenDashboard:
		return m.updateDashboard(key)
	case screenPicker:
		return m.updatePicker(message, key)
	case screenForm:
		return m.updateForm(message, key)
	case screenReview:
		return m.updateReview(key)
	case screenDeleteReview:
		return m.updateDeleteReview(key)
	case screenTimesheetReview:
		return m.updateTimesheetReview(key)
	case screenError:
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "r", "enter":
			m.screen = screenLoading
			m.loadingText = "Retrying ClickTime"
			m.lastError = nil
			return m, m.withSpinner(loadAllCmd(m.api, m.weekStart))
		}
	}
	return m, nil
}

func (m Model) updateComponent(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.screen == screenPicker {
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(message)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateDashboard(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.timesheetRows()
	switch key.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(rows)-1 {
			m.cursor++
		}
	case "left", "h":
		if m.dayCursor > 0 {
			m.dayCursor--
		}
	case "right", "l":
		if m.dayCursor < 6 {
			m.dayCursor++
		}
	case "[", "pgup", "shift+left":
		return m.changeWeek(-7)
	case "]", "pgdown", "shift+right":
		return m.changeWeek(7)
	case "t":
		start := startOfWeek(m.now())
		m.dayCursor = dayIndexInWeek(m.now(), start)
		if sameDay(start, m.weekStart) {
			return m, nil
		}
		m.weekStart = start
		m.cursor = 0
		m.screen = screenLoading
		m.loadingText = "Loading the current week"
		m.lastError = nil
		return m, m.withSpinner(loadEntriesCmd(m.api, start))
	case "r":
		m.screen = screenLoading
		m.loadingText = "Refreshing your week"
		m.lastError = nil
		return m, m.withSpinner(loadEntriesCmd(m.api, m.weekStart))
	case "s":
		date := m.selectedDate()
		m.screen = screenTimesheetLoading
		m.loadingText = "Loading timesheet for " + date.Format("Jan 2")
		m.status = ""
		m.lastError = nil
		m.timesheetToSubmit = clicktime.Timesheet{}
		m.submissionEntries = nil
		m.submissionTimeOff = nil
		m.attestationStatement = ""
		return m, m.withSpinner(loadTimesheetForSubmissionCmd(m.api, date))
	case "n":
		m.beginNewEntry(m.selectedDate())
	case "e", "enter":
		entries := m.selectedEntries()
		switch len(entries) {
		case 0:
			if m.cursor < 0 || m.cursor >= len(rows) {
				m.status = "No project row is selected. Press n to add one."
				return m, nil
			}
			m.beginNewEntryForRow(m.selectedDate(), rows[m.cursor])
		case 1:
			if entries[0].key() == "project:" || entries[0].key() == "timeoff:" {
				m.status = "ClickTime did not return an ID for that entry, so it cannot be edited."
				return m, nil
			}
			m.beginEditEntry(entries[0])
		default:
			m.openEntryPicker(entries)
		}
	case "d":
		entries := m.selectedEntries()
		if len(entries) == 0 {
			m.status = "There are no entries in that cell to delete."
			return m, nil
		}
		for _, entry := range entries {
			if entry.entryID() == "" {
				m.status = "ClickTime did not return an ID for that entry, so it cannot be deleted."
				return m, nil
			}
		}
		m.pendingDeleteEntries = append([]trackedEntry(nil), entries...)
		m.screen = screenDeleteReview
		m.status = ""
		m.lastError = nil
	}
	return m, nil
}

func (m Model) changeWeek(days int) (tea.Model, tea.Cmd) {
	m.weekStart = m.weekStart.AddDate(0, 0, days)
	m.cursor = 0
	m.screen = screenLoading
	m.loadingText = "Loading week of " + m.weekStart.Format("Jan 2")
	m.lastError = nil
	return m, m.withSpinner(loadEntriesCmd(m.api, m.weekStart))
}

func (m *Model) beginNewEntry(date time.Time) {
	m.status = ""
	m.lastError = nil
	m.draft = draft{date: date.Format(time.DateOnly)}
	m.availableTasks = nil
	m.openCategoryPicker()
}

func (m *Model) beginNewEntryForRow(date time.Time, row timesheetRow) {
	m.status = ""
	m.lastError = nil
	m.availableTasks = nil

	if row.kind == timeOffEntry {
		timeOffType := m.timeOffTypeByID(row.taskID)
		typeName := timeOffType.Label()
		if typeName == "" {
			typeName = row.task
		}
		m.draft = draft{
			kind: timeOffEntry, date: date.Format(time.DateOnly),
			timeOffTypeID: row.taskID, timeOffTypeName: typeName, returnDashboard: true,
		}
		m.openForm()
		return
	}

	job := m.jobByID(row.jobID)
	client := m.clientByID(job.ClientID)
	task := m.taskByID(row.taskID)
	clientName := client.Label()
	if clientName == "" {
		clientName = "—"
	}
	jobName := job.Label()
	if jobName == "" {
		jobName = row.project
	}
	taskName := task.Label()
	if taskName == "" {
		taskName = row.task
	}
	m.draft = draft{
		kind: projectEntry, date: date.Format(time.DateOnly),
		clientID: client.ID, clientName: clientName,
		jobID: row.jobID, jobName: jobName,
		taskID: row.taskID, taskName: taskName, returnDashboard: true,
	}
	m.openForm()
}

func (m *Model) openCategoryPicker() {
	items := []list.Item{
		pickerItem{id: "projects", title: "Projects", description: "Log time against a project and task"},
		pickerItem{id: "timeoff", title: "Time Off", description: "Vacation, sick leave, and other leave types"},
	}
	m.openPicker(pickerCategory, "What kind of time are you adding?", items)
}

func (m *Model) openEntryPicker(entries []trackedEntry) {
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		if entry.key() == "project:" || entry.key() == "timeoff:" {
			continue
		}
		title := fmt.Sprintf("%.2fh", entry.hours())
		if note := oneLine(entry.comment()); note != "" {
			title += " · " + note
		}
		items = append(items, pickerItem{id: entry.key(), title: title, description: displayDate(entry.date())})
	}
	if len(items) == 0 {
		m.status = "ClickTime did not return IDs for those entries, so they cannot be edited."
		return
	}
	m.openPicker(pickerEntry, "Choose an entry to edit", items)
}

func (m *Model) beginEditEntry(entry trackedEntry) {
	m.status = ""
	m.lastError = nil
	if entry.kind == timeOffEntry {
		timeOffType := m.timeOffTypeByID(entry.timeOff.TimeOffTypeID)
		m.draft = draft{
			kind: timeOffEntry, date: dateString(entry.timeOff.Date),
			hours: float64(entry.timeOff.Hours), comment: entry.timeOff.Notes,
			timeOffTypeID:   entry.timeOff.TimeOffTypeID,
			timeOffTypeName: timeOffType.Label(), entryID: entry.timeOff.Key(),
		}
		if m.draft.timeOffTypeName == "" {
			m.draft.timeOffTypeName = entry.timeOff.TimeOffTypeID
		}
		m.openForm()
		return
	}

	project := entry.project
	job := m.jobByID(project.JobID)
	client := m.clientByID(job.ClientID)
	task := m.taskByID(project.TaskID)
	m.draft = draft{
		kind: projectEntry, date: entryDate(project),
		hours: float64(project.Hours), comment: project.Comment,
		clientID: client.ID, clientName: client.Label(),
		jobID: project.JobID, jobName: job.Label(),
		taskID: project.TaskID, taskName: task.Label(), entryID: project.Key(),
	}
	if m.draft.jobName == "" {
		m.draft.jobName = project.JobID
	}
	if m.draft.taskName == "" {
		m.draft.taskName = project.TaskID
	}
	if m.draft.clientName == "" {
		m.draft.clientName = "—"
	}
	m.openForm()
}

func (m *Model) openPicker(kind pickerKind, title string, items []list.Item) {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	width, height := m.pickerSize()
	picker := list.New(items, delegate, width, height)
	picker.Title = title
	picker.SetShowHelp(false)
	picker.SetShowStatusBar(true)
	picker.SetFilteringEnabled(true)
	picker.Styles.Title = picker.Styles.Title.Background(accent).Foreground(lipgloss.Color("#071013"))
	m.picker = picker
	m.pickerKind = kind
	m.screen = screenPicker
}

func (m *Model) openJobPicker() {
	items := make([]list.Item, 0, len(m.jobs))
	for _, job := range m.jobs {
		client := m.clientByID(job.ClientID)
		description := client.Label()
		if job.JobNumber != "" {
			if description != "" {
				description += " · "
			}
			description += job.JobNumber
		}
		items = append(items, pickerItem{id: job.ID, title: job.Label(), description: description})
	}
	if len(items) == 0 {
		m.screen = screenDashboard
		m.status = "No active projects are available."
		return
	}
	m.openPicker(pickerJob, "Choose a project", items)
}

func (m *Model) openTimeOffPicker() {
	items := make([]list.Item, 0, len(m.timeOffTypes))
	for _, timeOffType := range m.timeOffTypes {
		if hiddenTimeOffType(timeOffType.Label()) {
			continue
		}
		items = append(items, pickerItem{id: timeOffType.Key(), title: timeOffType.Label()})
	}
	if len(items) == 0 {
		m.screen = screenDashboard
		m.status = "No time off options are available."
		return
	}
	m.openPicker(pickerTimeOff, "Choose a time off option", items)
}

func (m *Model) openTaskPicker(tasks []clicktime.Task) {
	items := make([]list.Item, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, pickerItem{id: task.ID, title: task.Label(), description: task.Code})
	}
	m.openPicker(pickerTask, "Choose a task", items)
}

func (m Model) updatePicker(message tea.Msg, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.picker.SettingFilter() {
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(message)
		return m, cmd
	}
	switch key.String() {
	case "esc":
		m.backFromPicker()
		return m, nil
	case "q":
		m.screen = screenDashboard
		m.lastError = nil
		return m, nil
	case "enter":
		selected, ok := m.picker.SelectedItem().(pickerItem)
		if !ok {
			return m, nil
		}
		switch m.pickerKind {
		case pickerCategory:
			switch selected.id {
			case "projects":
				m.draft.kind = projectEntry
				m.openJobPicker()
			case "timeoff":
				m.draft.kind = timeOffEntry
				m.openTimeOffPicker()
			}
			return m, nil
		case pickerJob:
			job := m.jobByID(selected.id)
			client := m.clientByID(job.ClientID)
			m.draft.jobID = selected.id
			m.draft.jobName = selected.title
			m.draft.clientID = client.ID
			m.draft.clientName = client.Label()
			m.availableTasks = nil
			m.screen = screenTaskLoading
			m.loadingText = "Loading tasks for " + selected.title
			m.lastError = nil
			return m, m.withSpinner(loadTasksCmd(m.api, selected.id))
		case pickerTask:
			m.draft.taskID = selected.id
			m.draft.taskName = selected.title
			m.openForm()
			return m, nil
		case pickerTimeOff:
			m.draft.timeOffTypeID = selected.id
			m.draft.timeOffTypeName = selected.title
			m.openForm()
			return m, nil
		case pickerEntry:
			entry, ok := m.entryByID(selected.id)
			if !ok {
				m.screen = screenDashboard
				m.status = "That entry is no longer available. Refresh and try again."
				return m, nil
			}
			m.beginEditEntry(entry)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(message)
	return m, cmd
}

func (m *Model) backFromPicker() {
	m.lastError = nil
	switch m.pickerKind {
	case pickerJob, pickerTimeOff:
		m.openCategoryPicker()
	case pickerTask:
		m.openJobPicker()
	default:
		m.screen = screenDashboard
	}
}

func (m *Model) backFromForm() {
	m.lastError = nil
	m.blurForm()
	if m.draft.entryID != "" || m.draft.returnDashboard {
		m.screen = screenDashboard
		return
	}
	if m.draft.kind == timeOffEntry {
		m.openTimeOffPicker()
		return
	}
	if len(m.availableTasks) > 0 {
		m.openTaskPicker(m.availableTasks)
		return
	}
	m.openJobPicker()
}

func (m *Model) openForm() {
	if m.draft.hours > 0 {
		m.hoursInput.SetValue(strconv.FormatFloat(m.draft.hours, 'f', -1, 64))
	} else {
		m.hoursInput.SetValue("")
	}
	m.notesInput.SetValue(m.draft.comment)
	m.formFocus = 0
	m.setFormFocus(0)
	m.screen = screenForm
	m.lastError = nil
}

func (m Model) updateForm(message tea.Msg, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.backFromForm()
		return m, nil
	case "tab":
		m.setFormFocus((m.formFocus + 1) % 3)
		return m, nil
	case "shift+tab":
		m.setFormFocus((m.formFocus + 2) % 3)
		return m, nil
	case "ctrl+r":
		return m.prepareReview()
	case "enter":
		if m.formFocus == 0 {
			m.setFormFocus(1)
			return m, nil
		}
		if m.formFocus == 1 && strings.TrimSpace(m.notesInput.Value()) == "" {
			return m.prepareReview()
		}
		if m.formFocus == 2 {
			return m.prepareReview()
		}
	}

	var cmd tea.Cmd
	switch m.formFocus {
	case 0:
		m.hoursInput, cmd = m.hoursInput.Update(message)
	case 1:
		m.notesInput, cmd = m.notesInput.Update(message)
	}
	return m, cmd
}

func (m Model) prepareReview() (tea.Model, tea.Cmd) {
	parsedDate, hours, err := validateForm(m.draft.date, m.hoursInput.Value())
	if err != nil {
		m.lastError = err
		return m, nil
	}
	m.draft.date = parsedDate
	m.draft.hours = hours
	m.draft.comment = strings.TrimSpace(m.notesInput.Value())
	m.lastError = nil
	m.blurForm()
	m.screen = screenReview
	return m, nil
}

func (m Model) updateReview(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "b", "esc":
		m.screen = screenForm
		m.setFormFocus(2)
		m.lastError = nil
	case "y", "enter":
		m.screen = screenSaving
		m.loadingText = "Saving your time entry"
		m.lastError = nil
		return m, m.withSpinner(saveEntryCmd(m.api, m.draft))
	}
	return m, nil
}

func (m Model) updateDeleteReview(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "b", "esc":
		m.screen = screenDashboard
		m.pendingDeleteEntries = nil
		m.lastError = nil
	case "y", "enter":
		if len(m.pendingDeleteEntries) == 0 {
			m.screen = screenDashboard
			m.status = "There are no entries in that cell to delete."
			m.lastError = nil
			return m, nil
		}
		m.screen = screenSaving
		m.loadingText = "Deleting selected cell"
		m.lastError = nil
		return m, m.withSpinner(deleteEntriesCmd(m.api, m.pendingDeleteEntries))
	}
	return m, nil
}

func (m Model) updateTimesheetReview(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "b", "esc":
		m.screen = screenDashboard
		m.lastError = nil
	case "y", "enter":
		if strings.TrimSpace(m.timesheetToSubmit.ID) == "" {
			m.lastError = fmt.Errorf("timesheet ID is missing; refresh and try again")
			return m, nil
		}
		m.screen = screenTimesheetSubmitting
		m.loadingText = "Submitting your timesheet"
		m.lastError = nil
		return m, m.withSpinner(submitTimesheetCmd(m.api, m.timesheetToSubmit.ID))
	}
	return m, nil
}

func (m *Model) setFormFocus(index int) {
	m.blurForm()
	m.formFocus = index
	switch index {
	case 0:
		_ = m.hoursInput.Focus()
	case 1:
		_ = m.notesInput.Focus()
	}
}

func (m *Model) blurForm() {
	m.hoursInput.Blur()
	m.notesInput.Blur()
}

func (m Model) View() string {
	if m.width > 0 && m.width < 42 {
		return m.appFrame("tuitime needs a terminal at least 42 columns wide.")
	}
	if m.screen == screenDashboard && m.width > 0 && m.width < 100 {
		return m.appFrame("The weekly table needs a terminal at least 100 columns wide.")
	}
	switch m.screen {
	case screenLoading, screenTaskLoading, screenSaving, screenTimesheetLoading, screenTimesheetSubmitting:
		return m.loadingView()
	case screenDashboard:
		return m.dashboardView()
	case screenPicker:
		return m.pickerView()
	case screenForm:
		return m.formView()
	case screenReview:
		return m.reviewView()
	case screenDeleteReview:
		return m.deleteReviewView()
	case screenTimesheetReview:
		return m.timesheetReviewView()
	case screenError:
		return m.errorView()
	default:
		return ""
	}
}

func (m Model) loadingView() string {
	text := m.loadingText
	if text == "" {
		text = "Loading"
	}
	return m.appFrame(titleStyle.Render("tuitime") + "\n\n" + m.spinner.View() + " " + text + "…")
}

func (m Model) dashboardView() string {
	weekEnd := m.weekStart.AddDate(0, 0, 6)
	name := m.me.DisplayName()
	if name == "" {
		name = "ClickTime"
	}

	rows := m.timesheetRows()
	tableWidth := m.timesheetTableWidth()
	projectWidth := tableWidth - 87
	headers := []string{"Project", "Task"}
	for day := 0; day < 7; day++ {
		date := m.weekStart.AddDate(0, 0, day)
		headers = append(headers, m.dayHeader(date))
	}
	headers = append(headers, "Total")

	data := make([][]string, 0, max(1, len(rows))+1)
	for _, row := range rows {
		values := []string{row.project, row.task}
		for day := 0; day < 7; day++ {
			values = append(values, formatTableHours(row.hours[day]))
		}
		values = append(values, formatTableHours(row.total))
		data = append(data, values)
	}
	if len(rows) == 0 {
		data = append(data, []string{"No time entries", "Press n to add", "·", "·", "·", "·", "·", "·", "·", "0.00"})
	}

	totals := []string{"Daily total", ""}
	for day := 0; day < 7; day++ {
		totals = append(totals, formatTableHours(m.totalForDate(m.weekStart.AddDate(0, 0, day))))
	}
	totals = append(totals, formatTableHours(m.weekTotal()))
	data = append(data, totals)
	totalRow := len(data) - 1

	timesheet := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(tableBorderStyle).
		BorderRow(true).
		BorderColumn(true).
		Headers(headers...).
		Rows(data...).
		Width(tableWidth).
		Wrap(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := tableCellStyle
			switch {
			case row == table.HeaderRow:
				style = tableHeaderStyle
			case row == totalRow:
				style = tableTotalStyle
			case row == m.cursor && col == m.dayCursor+2:
				style = tableSelectedCellStyle
			case row == m.cursor && col < 2:
				style = tableSelectedLabelStyle
			}
			if row == table.HeaderRow && col == m.dayCursor+2 {
				style = tableSelectedHeaderStyle
			}
			switch {
			case col == 0:
				return style.Width(projectWidth).Align(lipgloss.Left)
			case col == 1:
				return style.Width(12).Align(lipgloss.Left)
			case col >= 2 && col <= 8:
				return style.Padding(0).Width(8).Align(lipgloss.Center)
			default:
				return style.Padding(0, 1).Width(8).Align(lipgloss.Right)
			}
		})

	var body strings.Builder
	heading := titleStyle.Render("tuitime") + "  " + mutedStyle.Render(name)
	body.WriteString(lipgloss.NewStyle().Width(tableWidth).Render(heading))
	body.WriteString("\n")
	body.WriteString(subtitleStyle.Render(fmt.Sprintf("%s – %s", m.weekStart.Format("Jan 2"), weekEnd.Format("Jan 2, 2006"))))
	if statusLine := m.timesheetStatusLine(); statusLine != "" {
		body.WriteString("\n")
		body.WriteString(statusLine)
	}
	body.WriteString("\n\n")
	body.WriteString(timesheet.Render())
	body.WriteString("\n\n")
	body.WriteString(m.selectedCellDetail(rows))
	if m.status != "" {
		body.WriteString("\n\n" + successStyle.Render(m.status))
	}
	if m.lastError != nil {
		body.WriteString("\n\n" + errorStyle.Render(m.lastError.Error()))
	}
	body.WriteString("\n\n")
	body.WriteString(helpStyle.Render("hl/←→ day  jk/↑↓ row  [] week  n new  e edit  d delete  s submit  t today  r refresh  q quit"))
	body.WriteString("\n\n")
	body.WriteString(mutedStyle.Render("* today | + timesheet end | ※ today and timesheet end"))
	return m.appFrame(body.String())
}

func (m Model) pickerView() string {
	return m.appFrame(m.picker.View() + "\n" + helpStyle.Render("/ filter  enter choose  esc back  q cancel"))
}

func (m Model) formView() string {
	action := "New time entry"
	if m.draft.kind == timeOffEntry {
		action = "New time off entry"
	}
	if m.draft.entryID != "" {
		action = "Edit time entry"
		if m.draft.kind == timeOffEntry {
			action = "Edit time off entry"
		}
	}
	var body strings.Builder
	body.WriteString(titleStyle.Render(action) + "\n\n")
	body.WriteString(summaryLine("Date", displayDate(m.draft.date)) + "\n")
	if m.draft.kind == timeOffEntry {
		body.WriteString(summaryLine("Type", m.draft.timeOffTypeName) + "\n\n")
	} else {
		body.WriteString(summaryLine("Client", m.draft.clientName) + "\n")
		body.WriteString(summaryLine("Project", m.draft.jobName) + "\n")
		body.WriteString(summaryLine("Task", m.draft.taskName) + "\n\n")
	}
	body.WriteString(m.formField("Hours", m.hoursInput.View(), 0))
	body.WriteString("\n")
	body.WriteString(m.formField("Notes", m.notesInput.View(), 1))
	body.WriteString("\n")
	button := "  Review entry  "
	if m.formFocus == 2 {
		button = buttonActiveStyle.Render(button)
	} else {
		button = buttonStyle.Render(button)
	}
	body.WriteString(button)
	if m.lastError != nil {
		body.WriteString("\n\n" + errorStyle.Render(m.lastError.Error()))
	}
	body.WriteString("\n\n" + helpStyle.Render("tab next field  ctrl+r review  esc back"))
	return m.appFrame(body.String())
}

func (m Model) reviewView() string {
	action := "Create this entry?"
	if m.draft.kind == timeOffEntry {
		action = "Create this time off entry?"
	}
	if m.draft.entryID != "" {
		action = "Update this entry?"
		if m.draft.kind == timeOffEntry {
			action = "Update this time off entry?"
		}
	}
	comment := m.draft.comment
	if comment == "" {
		comment = "—"
	}
	var body strings.Builder
	body.WriteString(titleStyle.Render(action) + "\n\n")
	body.WriteString(summaryLine("Date", displayDate(m.draft.date)) + "\n")
	if m.draft.kind == timeOffEntry {
		body.WriteString(summaryLine("Type", m.draft.timeOffTypeName) + "\n")
	} else {
		body.WriteString(summaryLine("Client", m.draft.clientName) + "\n")
		body.WriteString(summaryLine("Project", m.draft.jobName) + "\n")
		body.WriteString(summaryLine("Task", m.draft.taskName) + "\n")
	}
	body.WriteString(summaryLine("Hours", strconv.FormatFloat(m.draft.hours, 'f', -1, 64)) + "\n")
	body.WriteString(summaryLine("Notes", comment))
	if m.lastError != nil {
		body.WriteString("\n\n" + errorStyle.Render(m.lastError.Error()))
	}
	body.WriteString("\n\n" + helpStyle.Render("y enter confirm  b esc back"))
	return m.appFrame(body.String())
}

func (m Model) deleteReviewView() string {
	entries := m.pendingDeleteEntries
	var body strings.Builder
	body.WriteString(titleStyle.Render("Delete selected cell?") + "\n\n")
	body.WriteString(summaryLine("Entries", strconv.Itoa(len(entries))) + "\n")
	body.WriteString(summaryLine("Hours", strconv.FormatFloat(totalTrackedHours(entries), 'f', 2, 64)))
	for _, entry := range entries {
		body.WriteString("\n")
		body.WriteString(mutedStyle.Render("  " + m.deleteEntrySummary(entry)))
	}
	if m.lastError != nil {
		body.WriteString("\n\n" + errorStyle.Render(m.lastError.Error()))
	}
	body.WriteString("\n\n" + helpStyle.Render("y enter delete  b esc cancel"))
	return m.appFrame(body.String())
}

func (m Model) deleteEntrySummary(entry trackedEntry) string {
	name := ""
	if entry.kind == timeOffEntry {
		timeOffType := m.timeOffTypeByID(entry.timeOff.TimeOffTypeID)
		name = "Time Off / " + firstDisplayValue(timeOffType.Label(), entry.timeOff.TimeOffTypeID)
	} else {
		project := entry.project
		job := m.jobByID(project.JobID)
		client := m.clientByID(job.ClientID)
		task := m.taskByID(project.TaskID)
		projectName := firstDisplayValue(job.Label(), project.JobID)
		if clientName := client.Label(); clientName != "" && clientName != projectName {
			projectName = clientName + " / " + projectName
		}
		taskName := firstDisplayValue(task.Label(), project.TaskID)
		name = projectName + " / " + taskName
	}

	summary := fmt.Sprintf("%s · %s · %.2fh", name, displayDate(entry.date()), entry.hours())
	if note := oneLine(entry.comment()); note != "" {
		summary += " · " + note
	}
	return summary
}

func firstDisplayValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (m Model) timesheetReviewView() string {
	timesheet := m.timesheetToSubmit
	status := strings.TrimSpace(timesheet.Status)
	if status == "" {
		status = "Unknown"
	}

	var body strings.Builder
	body.WriteString(titleStyle.Render("Submit this timesheet?") + "\n\n")
	body.WriteString(summaryLine("Period", displayDateRange(timesheet.StartDate, timesheet.EndDate)) + "\n")
	body.WriteString(summaryLine("Status", status) + "\n")
	body.WriteString(summaryLine("Hours", strconv.FormatFloat(timesheet.TotalHours(), 'f', 2, 64)))
	if calendar := m.timesheetCalendarView(timesheet); calendar != "" {
		body.WriteString("\n\n" + subtitleStyle.Render("Calendar"))
		body.WriteString("\n" + detailStyle.Render(calendar))
	}
	body.WriteString("\n\n" + subtitleStyle.Render("Submitting sends the entire period to ClickTime for approval."))
	body.WriteString("\n" + mutedStyle.Render("You may not be able to edit its entries after submission."))
	if statement := strings.TrimSpace(m.attestationStatement); statement != "" {
		body.WriteString("\n\n" + subtitleStyle.Render("Attestation"))
		body.WriteString("\n" + detailStyle.Width(76).Render(statement))
		body.WriteString("\n" + mutedStyle.Render("By confirming, you agree to this attestation."))
	} else {
		body.WriteString("\n\n" + detailStyle.Render("By confirming, you attest that this timesheet is complete and accurate."))
	}
	if m.lastError != nil {
		body.WriteString("\n\n" + errorStyle.Render(m.lastError.Error()))
	}
	body.WriteString("\n\n" + helpStyle.Render("y enter submit  b esc cancel"))
	return m.appFrame(body.String())
}

func (m Model) errorView() string {
	message := "Unable to load ClickTime."
	if m.lastError != nil {
		message = m.lastError.Error()
	}
	body := titleStyle.Render("tuitime") + "\n\n" + errorStyle.Render(message) + "\n\n" + helpStyle.Render("r enter retry  q quit")
	return m.appFrame(body)
}

func (m Model) formField(label, value string, index int) string {
	marker := "  "
	style := labelStyle
	if m.formFocus == index {
		marker = "› "
		style = activeLabelStyle
	}
	return marker + style.Render(label) + "\n  " + value + "\n"
}

func (m Model) timesheetRows() []timesheetRow {
	byKey := make(map[string]*timesheetRow)
	for _, entry := range m.entries {
		day := -1
		for index := 0; index < 7; index++ {
			if dateString(entry.Date) == m.weekStart.AddDate(0, 0, index).Format(time.DateOnly) {
				day = index
				break
			}
		}
		if day < 0 {
			continue
		}

		key := "project\x00" + entry.JobID + "\x00" + entry.TaskID
		row, ok := byKey[key]
		if !ok {
			job := m.jobByID(entry.JobID)
			client := m.clientByID(job.ClientID)
			task := m.taskByID(entry.TaskID)
			projectName := job.Label()
			if projectName == "" {
				projectName = entry.JobID
			}
			if clientName := client.Label(); clientName != "" && clientName != projectName {
				projectName = clientName + " / " + projectName
			}
			taskName := task.Label()
			if taskName == "" {
				taskName = entry.TaskID
			}
			row = &timesheetRow{
				kind: projectEntry, jobID: entry.JobID, taskID: entry.TaskID,
				project: projectName, task: taskName,
			}
			byKey[key] = row
		}
		row.entries[day] = append(row.entries[day], trackedEntry{kind: projectEntry, project: entry})
		row.hours[day] += float64(entry.Hours)
		row.total += float64(entry.Hours)
	}

	for _, entry := range m.timeOffEntries {
		day := dayForDate(entry.Date, m.weekStart)
		if day < 0 {
			continue
		}
		key := "timeoff\x00" + entry.TimeOffTypeID
		row, ok := byKey[key]
		if !ok {
			timeOffType := m.timeOffTypeByID(entry.TimeOffTypeID)
			typeName := timeOffType.Label()
			if typeName == "" {
				typeName = entry.TimeOffTypeID
			}
			row = &timesheetRow{
				kind: timeOffEntry, taskID: entry.TimeOffTypeID,
				project: "Time Off", task: typeName,
			}
			byKey[key] = row
		}
		row.entries[day] = append(row.entries[day], trackedEntry{kind: timeOffEntry, timeOff: entry})
		row.hours[day] += float64(entry.Hours)
		row.total += float64(entry.Hours)
	}

	rows := make([]timesheetRow, 0, len(byKey))
	for _, row := range byKey {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].kind != rows[j].kind {
			return rows[i].kind < rows[j].kind
		}
		if rows[i].project == rows[j].project {
			return rows[i].task < rows[j].task
		}
		return rows[i].project < rows[j].project
	})
	return rows
}

func (m Model) selectedDate() time.Time {
	return m.weekStart.AddDate(0, 0, min(6, max(0, m.dayCursor)))
}

func (m Model) dayHeader(date time.Time) string {
	header := date.Format("Mon 02")
	today := sameDay(date, m.now())
	timesheetEnd := m.isTimesheetEnd(date)
	switch {
	case today && timesheetEnd:
		return header + "※"
	case today:
		return header + "*"
	case timesheetEnd:
		return header + "+"
	default:
		return header
	}
}

func (m Model) isTimesheetEnd(date time.Time) bool {
	for _, timesheet := range m.timesheets {
		if dateString(timesheet.EndDate) == date.Format(time.DateOnly) {
			return true
		}
	}
	return false
}

func (m Model) timesheetStatusLine() string {
	if len(m.timesheets) == 0 {
		return ""
	}

	timesheets := append([]clicktime.Timesheet(nil), m.timesheets...)
	sort.SliceStable(timesheets, func(i, j int) bool {
		return dateString(timesheets[i].StartDate) < dateString(timesheets[j].StartDate)
	})

	selectedDate := m.selectedDate()
	items := make([]string, 0, len(timesheets))
	for _, timesheet := range timesheets {
		periodStyle := mutedStyle
		prefix := "  "
		if timesheetContainsDate(timesheet, selectedDate) {
			periodStyle = activeLabelStyle
			prefix = "› "
		}
		status, statusStyle := timesheetStatus(timesheet)
		items = append(items, periodStyle.Render(prefix+compactTimesheetPeriod(timesheet))+"  "+statusStyle.Render(status))
	}

	separator := mutedStyle.Render("  |  ")
	return labelStyle.Render("Timesheets") + "  " + strings.Join(items, separator)
}

func timesheetStatus(timesheet clicktime.Timesheet) (string, lipgloss.Style) {
	switch strings.ToLower(strings.TrimSpace(timesheet.Status)) {
	case "open":
		return "○ Open", warningStyle
	case "waiting", "submitted", "pending":
		return "◷ Submitted · awaiting approval", pendingStyle
	case "approved":
		return "✓ Approved", successStyle
	case "rejected":
		return "! Rejected", errorStyle
	}

	if status := strings.TrimSpace(timesheet.Status); status != "" {
		return "? " + status, mutedStyle
	}
	if hasTimesheetAction(timesheet.Actions, "Submit") {
		return "○ Open", warningStyle
	}
	if hasTimesheetAction(timesheet.Actions, "UndoSubmit") || timesheet.HasBeenSubmitted || strings.TrimSpace(timesheet.SubmittedDate) != "" {
		return "◷ Submitted", pendingStyle
	}
	return "? Unknown", mutedStyle
}

func timesheetContainsDate(timesheet clicktime.Timesheet, date time.Time) bool {
	start, startErr := time.Parse(time.DateOnly, dateString(timesheet.StartDate))
	end, endErr := time.Parse(time.DateOnly, dateString(timesheet.EndDate))
	selected, selectedErr := time.Parse(time.DateOnly, date.Format(time.DateOnly))
	return startErr == nil && endErr == nil && selectedErr == nil && !selected.Before(start) && !selected.After(end)
}

func compactTimesheetPeriod(timesheet clicktime.Timesheet) string {
	start, startErr := time.Parse(time.DateOnly, dateString(timesheet.StartDate))
	end, endErr := time.Parse(time.DateOnly, dateString(timesheet.EndDate))
	if startErr != nil || endErr != nil {
		return displayDateRange(timesheet.StartDate, timesheet.EndDate)
	}
	switch {
	case sameDay(start, end):
		return start.Format("Jan 2")
	case start.Year() == end.Year() && start.Month() == end.Month():
		return fmt.Sprintf("%s %d–%d", start.Format("Jan"), start.Day(), end.Day())
	case start.Year() == end.Year():
		return start.Format("Jan 2") + "–" + end.Format("Jan 2")
	default:
		return start.Format("Jan 2, 2006") + "–" + end.Format("Jan 2, 2006")
	}
}

func (m Model) timesheetCalendarView(timesheet clicktime.Timesheet) string {
	start, startErr := time.Parse(time.DateOnly, dateString(timesheet.StartDate))
	end, endErr := time.Parse(time.DateOnly, dateString(timesheet.EndDate))
	if startErr != nil || endErr != nil || end.Before(start) {
		return ""
	}

	codes, hoursByDateCode, totalsByDate := m.submissionChargeCodeHours()
	if len(codes) == 0 {
		codes = []submissionChargeCode{{key: "total", alias: "Code1", name: "Total hours", total: timesheet.TotalHours()}}
		hoursByDateCode = map[string]map[string]float64{}
		totalsByDate = map[string]float64{}
		for _, total := range timesheet.DayTotals {
			if date := dateString(total.Date); date != "" {
				hours := float64(total.Hours)
				hoursByDateCode[date] = map[string]float64{"total": hours}
				totalsByDate[date] = hours
			}
		}
	}

	weekViews := make([]string, 0)
	hasEmptyWeekday := false
	for weekStart := startOfWeek(start); !weekStart.After(end); weekStart = weekStart.AddDate(0, 0, 7) {
		weekEnd := weekStart.AddDate(0, 0, 6)
		headers := []string{"Date"}
		for _, code := range codes {
			headers = append(headers, code.alias)
		}

		rows := make([][]string, 0, 7)
		emptyRows := make([]bool, 0, 7)
		for index := 0; index < 7; index++ {
			date := weekStart.AddDate(0, 0, index)
			inPeriod := !date.Before(start) && !date.After(end)
			dateKey := date.Format(time.DateOnly)
			row := []string{date.Format("Mon Jan 02")}
			emptyWeekday := inPeriod && isWeekday(date) && totalsByDate[dateKey] == 0
			hasEmptyWeekday = hasEmptyWeekday || emptyWeekday
			for _, code := range codes {
				value := ""
				if inPeriod {
					value = formatTableHours(hoursByDateCode[dateKey][code.key])
				}
				row = append(row, value)
			}
			rows = append(rows, row)
			emptyRows = append(emptyRows, emptyWeekday)
		}

		weekViews = append(weekViews, submissionWeekBlock(fmt.Sprintf("%s - %s", weekStart.Format("Jan 2"), weekEnd.Format("Jan 2")), headers, rows, emptyRows))
	}

	var body strings.Builder
	body.WriteString(joinSubmissionWeekBlocks(weekViews))
	body.WriteString("\n\n")
	for _, code := range codes {
		body.WriteString(fmt.Sprintf("%s - %sh - %s\n", code.alias, strconv.FormatFloat(code.total, 'f', 2, 64), code.name))
	}
	if hasEmptyWeekday {
		body.WriteString(emptyDayStyle.Render("Empty"))
		body.WriteString(" - These are weekdays without any time entry. FTO need not be included in ClickTime.\n")
	}
	return strings.TrimRight(body.String(), "\n")
}

func (m Model) submissionChargeCodeHours() ([]submissionChargeCode, map[string]map[string]float64, map[string]float64) {
	codeByKey := make(map[string]*submissionChargeCode)
	hoursByDateCode := make(map[string]map[string]float64)
	totalsByDate := make(map[string]float64)

	addCode := func(key, name, date string, hours float64) {
		code, ok := codeByKey[key]
		if !ok {
			code = &submissionChargeCode{key: key, name: name}
			codeByKey[key] = code
		}
		code.total += hours
		if hoursByDateCode[date] == nil {
			hoursByDateCode[date] = make(map[string]float64)
		}
		hoursByDateCode[date][key] += hours
		totalsByDate[date] += hours
	}

	for _, entry := range m.submissionEntries {
		date := dateString(entry.Date)
		if date == "" {
			continue
		}
		key := "project\x00" + entry.JobID + "\x00" + entry.TaskID
		job := m.jobByID(entry.JobID)
		client := m.clientByID(job.ClientID)
		task := m.taskByID(entry.TaskID)
		projectName := job.Label()
		if projectName == "" {
			projectName = entry.JobID
		}
		if clientName := client.Label(); clientName != "" && clientName != projectName {
			projectName = clientName + " / " + projectName
		}
		taskName := task.Label()
		if taskName == "" {
			taskName = entry.TaskID
		}
		addCode(key, projectName+" / "+taskName, date, float64(entry.Hours))
	}

	for _, entry := range m.submissionTimeOff {
		date := dateString(entry.Date)
		if date == "" {
			continue
		}
		timeOffType := m.timeOffTypeByID(entry.TimeOffTypeID)
		typeName := timeOffType.Label()
		if typeName == "" {
			typeName = entry.TimeOffTypeID
		}
		key := "timeoff\x00" + entry.TimeOffTypeID
		addCode(key, "Time Off / "+typeName, date, float64(entry.Hours))
	}

	codes := make([]submissionChargeCode, 0, len(codeByKey))
	for _, code := range codeByKey {
		codes = append(codes, *code)
	}
	sort.Slice(codes, func(i, j int) bool {
		return codes[i].name < codes[j].name
	})
	for index := range codes {
		codes[index].alias = fmt.Sprintf("Code%d", index+1)
	}
	return codes, hoursByDateCode, totalsByDate
}

func joinSubmissionWeekBlocks(weekViews []string) string {
	if len(weekViews) == 0 {
		return ""
	}
	if len(weekViews) == 1 {
		return weekViews[0]
	}

	linesByWeek := make([][]string, 0, len(weekViews))
	maxHeight := 0
	widths := make([]int, 0, len(weekViews))
	for _, week := range weekViews {
		lines := strings.Split(week, "\n")
		linesByWeek = append(linesByWeek, lines)
		maxHeight = max(maxHeight, len(lines))
		widths = append(widths, lipgloss.Width(week))
	}

	var body strings.Builder
	for lineIndex := 0; lineIndex < maxHeight; lineIndex++ {
		if lineIndex > 0 {
			body.WriteString("\n")
		}
		for weekIndex, lines := range linesByWeek {
			if weekIndex > 0 {
				body.WriteString(tableBorderStyle.Render(" │ "))
			}
			line := ""
			if lineIndex < len(lines) {
				line = lines[lineIndex]
			}
			body.WriteString(lipgloss.NewStyle().Width(widths[weekIndex]).Render(line))
		}
	}
	return body.String()
}

func submissionWeekBlock(title string, headers []string, rows [][]string, emptyRows []bool) string {
	dateWidth := 12
	codeWidth := 9
	var body strings.Builder
	body.WriteString(subtitleStyle.Width(dateWidth + codeWidth*(len(headers)-1)).Render(title))
	body.WriteString("\n")
	for index, header := range headers {
		width := codeWidth
		align := lipgloss.Right
		if index == 0 {
			width = dateWidth
			align = lipgloss.Left
		}
		body.WriteString(tableHeaderStyle.Padding(0).Width(width).Align(align).Render(header))
	}
	for rowIndex, row := range rows {
		body.WriteString("\n")
		for colIndex, value := range row {
			width := codeWidth
			align := lipgloss.Right
			style := tableCellStyle.Padding(0).Width(width).Align(align)
			if colIndex == 0 {
				if rowIndex < len(emptyRows) && emptyRows[rowIndex] {
					body.WriteString(emptyDayStyle.Render(value))
					body.WriteString(strings.Repeat(" ", max(0, dateWidth-lipgloss.Width(value))))
					continue
				}
				width = dateWidth
				align = lipgloss.Left
				style = tableCellStyle.Padding(0).Width(width).Align(align)
			}
			body.WriteString(style.Render(value))
		}
	}
	return body.String()
}

func isWeekday(date time.Time) bool {
	return date.Weekday() >= time.Monday && date.Weekday() <= time.Friday
}

func hasTimesheetAction(actions []clicktime.TimesheetAction, action string) bool {
	for _, available := range actions {
		if strings.EqualFold(strings.TrimSpace(available.Action), strings.TrimSpace(action)) {
			return true
		}
	}
	return false
}

func (m Model) selectedEntries() []trackedEntry {
	rows := m.timesheetRows()
	if m.cursor < 0 || m.cursor >= len(rows) || m.dayCursor < 0 || m.dayCursor > 6 {
		return nil
	}
	return append([]trackedEntry(nil), rows[m.cursor].entries[m.dayCursor]...)
}

func (m Model) selectedCellDetail(rows []timesheetRow) string {
	date := m.selectedDate().Format("Mon, Jan 2")
	if len(rows) == 0 || m.cursor < 0 || m.cursor >= len(rows) {
		return detailStyle.Width(m.timesheetTableWidth()).Render(
			activeLabelStyle.Render(date) + mutedStyle.Render("  No entry — press n to add time"),
		)
	}
	row := rows[m.cursor]
	entries := row.entries[m.dayCursor]
	prefix := activeLabelStyle.Render(date) + "  " + row.project + " / " + row.task
	if len(entries) == 0 {
		return detailStyle.Width(m.timesheetTableWidth()).Render(prefix + mutedStyle.Render("  No entry — press e to add here"))
	}
	if len(entries) == 1 {
		detail := fmt.Sprintf("  %.2fh", entries[0].hours())
		if note := oneLine(entries[0].comment()); note != "" {
			detail += "  ·  " + note
		}
		return detailStyle.Width(m.timesheetTableWidth()).Render(prefix + detail)
	}
	return detailStyle.Width(m.timesheetTableWidth()).Render(
		fmt.Sprintf("%s  %.2fh across %d entries — press e to choose one", prefix, row.hours[m.dayCursor], len(entries)),
	)
}

func (m Model) timesheetTableWidth() int {
	if m.width <= 0 {
		return 112
	}
	return min(122, max(96, m.width-8))
}

func formatTableHours(hours float64) string {
	if hours == 0 {
		return "·"
	}
	return strconv.FormatFloat(hours, 'f', 2, 64)
}

func hiddenTimeOffType(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "fto", "military", "medical", "parental":
		return true
	default:
		return false
	}
}

func (m Model) totalForDate(date time.Time) float64 {
	var total float64
	for _, entry := range m.entries {
		if dateString(entry.Date) == date.Format(time.DateOnly) {
			total += float64(entry.Hours)
		}
	}
	for _, entry := range m.timeOffEntries {
		if dateString(entry.Date) == date.Format(time.DateOnly) {
			total += float64(entry.Hours)
		}
	}
	return total
}

func (m Model) weekTotal() float64 {
	var total float64
	for day := 0; day < 7; day++ {
		total += m.totalForDate(m.weekStart.AddDate(0, 0, day))
	}
	return total
}

func (m Model) clientByID(id string) clicktime.ClientResource {
	for _, client := range m.clients {
		if client.ID == id {
			return client
		}
	}
	return clicktime.ClientResource{}
}

func (m Model) jobByID(id string) clicktime.Job {
	for _, job := range m.jobs {
		if job.ID == id {
			return job
		}
	}
	return clicktime.Job{}
}

func (m Model) taskByID(id string) clicktime.Task {
	for _, task := range m.tasks {
		if task.ID == id {
			return task
		}
	}
	return clicktime.Task{}
}

func (m Model) timeOffTypeByID(id string) clicktime.TimeOffType {
	for _, timeOffType := range m.timeOffTypes {
		if timeOffType.Key() == id {
			return timeOffType
		}
	}
	return clicktime.TimeOffType{}
}

func (m Model) entryByID(id string) (trackedEntry, bool) {
	if strings.HasPrefix(id, "timeoff:") {
		key := strings.TrimPrefix(id, "timeoff:")
		for _, entry := range m.timeOffEntries {
			if entry.Key() == key {
				return trackedEntry{kind: timeOffEntry, timeOff: entry}, true
			}
		}
		return trackedEntry{}, false
	}
	key := strings.TrimPrefix(id, "project:")
	for _, entry := range m.entries {
		if entry.Key() == key {
			return trackedEntry{kind: projectEntry, project: entry}, true
		}
	}
	return trackedEntry{}, false
}

func (m *Model) resize() {
	if m.screen == screenPicker {
		width, height := m.pickerSize()
		m.picker.SetSize(width, height)
	}
	notesWidth := max(30, min(76, m.width-8))
	m.notesInput.SetWidth(notesWidth)
}

func (m Model) pickerSize() (int, int) {
	width := max(40, min(82, m.width-8))
	height := max(10, min(24, m.height-6))
	return width, height
}

func loadAllCmd(api *clicktime.Client, week time.Time) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		weekEnd := week.AddDate(0, 0, 6)
		me, err := api.Me(ctx)
		if err != nil {
			return operationErrorMsg{op: "initial load", err: err}
		}
		clients, err := api.Clients(ctx)
		if err != nil {
			return operationErrorMsg{op: "initial load", err: err}
		}
		jobs, err := api.Jobs(ctx)
		if err != nil {
			return operationErrorMsg{op: "initial load", err: err}
		}
		tasks, err := api.Tasks(ctx)
		if err != nil {
			return operationErrorMsg{op: "initial load", err: err}
		}
		timeOffTypes, err := api.TimeOffTypes(ctx)
		if err != nil {
			return operationErrorMsg{op: "initial load", err: err}
		}
		timesheets, err := api.Timesheets(ctx, week, weekEnd)
		if err != nil {
			return operationErrorMsg{op: "initial load", err: err}
		}
		entries, err := api.TimeEntries(ctx, week, weekEnd)
		if err != nil {
			return operationErrorMsg{op: "initial load", err: err}
		}
		timeOffEntries, err := api.TimeOff(ctx, week, weekEnd)
		if err != nil {
			return operationErrorMsg{op: "initial load", err: err}
		}
		return allDataMsg{
			me: me, clients: clients, jobs: jobs, tasks: tasks, timeOffTypes: timeOffTypes,
			entries: entries, timeOffEntries: timeOffEntries, timesheets: timesheets, week: week,
		}
	}
}

func loadEntriesCmd(api *clicktime.Client, week time.Time) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		weekEnd := week.AddDate(0, 0, 6)
		timesheets, err := api.Timesheets(ctx, week, weekEnd)
		if err != nil {
			return operationErrorMsg{op: "load week", err: err}
		}
		entries, err := api.TimeEntries(ctx, week, weekEnd)
		if err != nil {
			return operationErrorMsg{op: "load week", err: err}
		}
		timeOffEntries, err := api.TimeOff(ctx, week, weekEnd)
		if err != nil {
			return operationErrorMsg{op: "load week", err: err}
		}
		return entriesMsg{entries: entries, timeOffEntries: timeOffEntries, timesheets: timesheets, week: week}
	}
}

func loadTimesheetForSubmissionCmd(api *clicktime.Client, date time.Time) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		timesheet, err := api.TimesheetForDate(ctx, date)
		if err != nil {
			return operationErrorMsg{op: "load timesheet", err: err}
		}
		if strings.TrimSpace(timesheet.ID) == "" {
			return operationErrorMsg{op: "load timesheet", err: fmt.Errorf("ClickTime did not return a timesheet ID for %s", date.Format(time.DateOnly))}
		}
		start, startErr := time.Parse(time.DateOnly, dateString(timesheet.StartDate))
		end, endErr := time.Parse(time.DateOnly, dateString(timesheet.EndDate))
		if startErr != nil || endErr != nil || end.Before(start) {
			return operationErrorMsg{op: "load timesheet", err: fmt.Errorf("ClickTime returned an invalid timesheet period: %s", displayDateRange(timesheet.StartDate, timesheet.EndDate))}
		}
		entries, err := api.TimeEntries(ctx, start, end)
		if err != nil {
			return operationErrorMsg{op: "load timesheet", err: err}
		}
		timeOffEntries, err := api.TimeOff(ctx, start, end)
		if err != nil {
			return operationErrorMsg{op: "load timesheet", err: err}
		}
		actions, err := api.TimesheetActions(ctx, timesheet.ID)
		if err != nil {
			return operationErrorMsg{op: "load timesheet", err: err}
		}
		company, err := api.Company(ctx)
		if err != nil {
			return operationErrorMsg{op: "load timesheet", err: err}
		}
		return timesheetReadyMsg{
			timesheet: timesheet, actions: actions,
			attestationStatement: company.AttestationStatement,
			entries:              entries,
			timeOffEntries:       timeOffEntries,
		}
	}
}

func submitTimesheetCmd(api *clicktime.Client, timesheetID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		timesheet, err := api.SubmitTimesheet(ctx, timesheetID, clicktime.TimesheetSubmitInput{HasAttestation: true})
		if err != nil {
			return operationErrorMsg{op: "submit timesheet", err: err}
		}
		return timesheetSubmittedMsg{timesheet: timesheet}
	}
}

func loadTasksCmd(api *clicktime.Client, jobID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		tasks, err := api.TasksForJob(ctx, jobID)
		if err != nil {
			return operationErrorMsg{op: "load tasks", err: err}
		}
		return tasksMsg{tasks: tasks}
	}
}

func saveEntryCmd(api *clicktime.Client, value draft) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		var err error
		if value.kind == timeOffEntry {
			if value.entryID == "" {
				_, err = api.CreateTimeOff(ctx, clicktime.TimeOffInput{
					Date: value.date, Hours: value.hours,
					TimeOffTypeID: value.timeOffTypeID, Notes: value.comment,
				})
			} else {
				_, err = api.UpdateTimeOff(ctx, value.entryID, clicktime.TimeOffUpdateInput{
					Hours: value.hours, TimeOffTypeID: value.timeOffTypeID, Notes: value.comment,
				})
			}
		} else {
			input := clicktime.TimeEntryInput{
				Date: value.date, Hours: value.hours, JobID: value.jobID,
				TaskID: value.taskID, Comment: value.comment,
			}
			if value.entryID == "" {
				_, err = api.CreateTimeEntry(ctx, input)
			} else {
				_, err = api.UpdateTimeEntry(ctx, value.entryID, input)
			}
		}
		if err != nil {
			return operationErrorMsg{op: "save time entry", err: err}
		}
		return savedMsg{}
	}
}

func deleteEntriesCmd(api *clicktime.Client, entries []trackedEntry) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		for _, entry := range entries {
			var err error
			if entry.kind == timeOffEntry {
				err = api.DeleteTimeOff(ctx, entry.entryID())
			} else {
				err = api.DeleteTimeEntry(ctx, entry.entryID())
			}
			if err != nil {
				return operationErrorMsg{op: "delete time entry", err: err}
			}
		}
		status := "Selected cell deleted."
		if len(entries) == 1 {
			status = "Entry deleted."
		}
		return savedMsg{status: status}
	}
}

func validateForm(dateValue, hoursValue string) (string, float64, error) {
	dateValue = strings.TrimSpace(dateValue)
	date, err := time.Parse(time.DateOnly, dateValue)
	if err != nil {
		return "", 0, fmt.Errorf("date must use YYYY-MM-DD")
	}
	hours, err := strconv.ParseFloat(strings.TrimSpace(hoursValue), 64)
	if err != nil {
		return "", 0, fmt.Errorf("hours must be a number")
	}
	if math.IsNaN(hours) || math.IsInf(hours, 0) || hours <= 0 || hours > 24 {
		return "", 0, fmt.Errorf("hours must be greater than 0 and no more than 24")
	}
	return date.Format(time.DateOnly), hours, nil
}

func startOfWeek(value time.Time) time.Time {
	value = time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	daysSinceMonday := (int(value.Weekday()) + 6) % 7
	return value.AddDate(0, 0, -daysSinceMonday)
}

func dayIndexInWeek(value, weekStart time.Time) int {
	day := dayForDate(value.Format(time.DateOnly), weekStart)
	if day < 0 {
		return 0
	}
	return day
}

func dayForDate(value string, weekStart time.Time) int {
	for day := 0; day < 7; day++ {
		if dateString(value) == weekStart.AddDate(0, 0, day).Format(time.DateOnly) {
			return day
		}
	}
	return -1
}

func sortedEntries(entries []clicktime.TimeEntry) []clicktime.TimeEntry {
	result := append([]clicktime.TimeEntry(nil), entries...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Date == result[j].Date {
			return result[i].Key() < result[j].Key()
		}
		return result[i].Date < result[j].Date
	})
	return result
}

func sortedTimeOffEntries(entries []clicktime.TimeOffEntry) []clicktime.TimeOffEntry {
	result := append([]clicktime.TimeOffEntry(nil), entries...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Date == result[j].Date {
			return result[i].Key() < result[j].Key()
		}
		return result[i].Date < result[j].Date
	})
	return result
}

func resolveTasks(tasks, catalog []clicktime.Task) []clicktime.Task {
	byID := make(map[string]clicktime.Task, len(catalog))
	for _, task := range catalog {
		byID[task.ID] = task
	}
	resolved := make([]clicktime.Task, 0, len(tasks))
	for _, task := range tasks {
		if full, ok := byID[task.ID]; ok {
			task = mergeTask(full, task)
		}
		resolved = append(resolved, task)
	}
	return resolved
}

func mergeTasks(existing, incoming []clicktime.Task) []clicktime.Task {
	byID := make(map[string]clicktime.Task, len(existing)+len(incoming))
	for _, task := range existing {
		byID[task.ID] = task
	}
	for _, task := range incoming {
		if current, ok := byID[task.ID]; ok {
			task = mergeTask(current, task)
		}
		byID[task.ID] = task
	}
	result := make([]clicktime.Task, 0, len(byID))
	for _, task := range byID {
		result = append(result, task)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Label() < result[j].Label() })
	return result
}

func mergeTask(current, update clicktime.Task) clicktime.Task {
	if update.ID == "" {
		update.ID = current.ID
	}
	if update.Name == "" {
		update.Name = current.Name
	}
	if update.DisplayName == "" {
		update.DisplayName = current.DisplayName
	}
	if update.Code == "" {
		update.Code = current.Code
	}
	if !update.IsActive {
		update.IsActive = current.IsActive
	}
	return update
}

func entryDate(entry clicktime.TimeEntry) string {
	if date := dateString(entry.Date); date != "" {
		return date
	}
	return entry.Date
}

func dateString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len(time.DateOnly) {
		return value[:len(time.DateOnly)]
	}
	return value
}

func displayDate(value string) string {
	date, err := time.Parse(time.DateOnly, dateString(value))
	if err != nil {
		return value
	}
	return date.Format("Mon, Jan 2, 2006")
}

func displayDateRange(startValue, endValue string) string {
	start, startErr := time.Parse(time.DateOnly, dateString(startValue))
	end, endErr := time.Parse(time.DateOnly, dateString(endValue))
	if startErr != nil || endErr != nil {
		return displayDate(startValue) + " – " + displayDate(endValue)
	}
	switch {
	case start.Year() == end.Year() && start.Month() == end.Month():
		return fmt.Sprintf("%s %d–%d, %d", start.Format("Jan"), start.Day(), end.Day(), end.Year())
	case start.Year() == end.Year():
		return fmt.Sprintf("%s – %s, %d", start.Format("Jan 2"), end.Format("Jan 2"), end.Year())
	default:
		return start.Format("Jan 2, 2006") + " – " + end.Format("Jan 2, 2006")
	}
}

func sameDay(a, b time.Time) bool {
	return a.Format(time.DateOnly) == b.Format(time.DateOnly)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func summaryLine(label, value string) string {
	if strings.TrimSpace(value) == "" {
		value = "—"
	}
	return labelStyle.Render(fmt.Sprintf("%-9s", label)) + " " + value
}
