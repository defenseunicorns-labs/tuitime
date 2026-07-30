package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tuitime/internal/clicktime"
)

func TestStartOfWeek(t *testing.T) {
	t.Parallel()
	location := time.FixedZone("test", -7*60*60)
	tests := []struct {
		name string
		date time.Time
		want string
	}{
		{name: "Tuesday", date: time.Date(2026, time.July, 28, 14, 30, 0, 0, location), want: "2026-07-27"},
		{name: "Monday", date: time.Date(2026, time.July, 27, 8, 0, 0, 0, location), want: "2026-07-27"},
		{name: "Sunday", date: time.Date(2026, time.August, 2, 8, 0, 0, 0, location), want: "2026-07-27"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := startOfWeek(test.date)
			if got.Format(time.DateOnly) != test.want {
				t.Fatalf("startOfWeek() = %s, want %s", got.Format(time.DateOnly), test.want)
			}
			if got.Hour() != 0 || got.Location() != location {
				t.Fatalf("startOfWeek() = %v, want midnight in original location", got)
			}
		})
	}
}

func TestValidateForm(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		date    string
		hours   string
		wantErr bool
	}{
		{name: "valid decimal", date: "2026-07-28", hours: "7.5"},
		{name: "bad date", date: "07/28/2026", hours: "7.5", wantErr: true},
		{name: "zero", date: "2026-07-28", hours: "0", wantErr: true},
		{name: "too many", date: "2026-07-28", hours: "25", wantErr: true},
		{name: "not a number", date: "2026-07-28", hours: "lots", wantErr: true},
		{name: "NaN", date: "2026-07-28", hours: "NaN", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateForm(test.date, test.hours)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateForm() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestTimesheetRowsAndTotals(t *testing.T) {
	t.Parallel()
	week := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	model := Model{
		weekStart: week,
		dayCursor: 1,
		jobs: []clicktime.Job{
			{ID: "job-1", ClientID: "client-1", Name: "Apollo"},
			{ID: "job-2", ClientID: "client-1", Name: "Zeus"},
		},
		clients: []clicktime.ClientResource{{ID: "client-1", Name: "Space"}},
		tasks: []clicktime.Task{
			{ID: "task-1", Name: "Labor"},
			{ID: "task-2", Name: "Review"},
		},
		entries: []clicktime.TimeEntry{
			{ID: "2", Date: "2026-07-27", Hours: 2.5, JobID: "job-1", TaskID: "task-1"},
			{ID: "1", Date: "2026-07-28T00:00:00Z", Hours: 4, JobID: "job-1", TaskID: "task-1"},
			{ID: "3", Date: "2026-07-27", Hours: 1, JobID: "job-2", TaskID: "task-2"},
			{ID: "4", Date: "2026-08-03", Hours: 200, JobID: "job-1", TaskID: "task-1"},
		},
	}
	rows := model.timesheetRows()
	if len(rows) != 2 {
		t.Fatalf("len(timesheetRows()) = %d, want 2", len(rows))
	}
	if rows[0].project != "Space / Apollo" || rows[0].task != "Labor" {
		t.Fatalf("first row = %#v", rows[0])
	}
	if rows[0].hours[0] != 2.5 || rows[0].hours[1] != 4 || rows[0].total != 6.5 {
		t.Fatalf("first row hours = %#v, total = %v", rows[0].hours, rows[0].total)
	}
	selected := model.selectedEntries()
	if len(selected) != 1 || selected[0].project.ID != "1" {
		t.Fatalf("selectedEntries() = %#v", selected)
	}
	if got := model.weekTotal(); got != 7.5 {
		t.Fatalf("weekTotal() = %v, want 7.5", got)
	}
}

func TestNewEntryCategoryFlow(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)
	base := NewAt(nil, func() time.Time { return date })
	base.clients = []clicktime.ClientResource{{ID: "client-1", Name: "Space"}}
	base.jobs = []clicktime.Job{{ID: "job-1", ClientID: "client-1", Name: "OMEN.Delivery"}}
	base.timeOffTypes = []clicktime.TimeOffType{
		{ID: "vacation", Name: "Vacation"},
		{ID: "fto", Name: "FTO"},
		{ID: "fto-no-approval", Name: "FTO - No Approval Required"},
		{ID: "military", Name: "Military"},
		{ID: "medical", Name: "Medical"},
		{ID: "parental", Name: "Parental"},
	}
	base.beginNewEntry(date)
	if base.pickerKind != pickerCategory || len(base.picker.Items()) != 2 {
		t.Fatalf("initial picker kind = %v, items = %#v", base.pickerKind, base.picker.Items())
	}

	projects := base
	projects.picker.Select(0)
	updated, _ := projects.updatePicker(tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter})
	projectModel := updated.(Model)
	if projectModel.pickerKind != pickerJob || len(projectModel.picker.Items()) != 1 {
		t.Fatalf("project picker kind = %v, items = %#v", projectModel.pickerKind, projectModel.picker.Items())
	}
	project := projectModel.picker.Items()[0].(pickerItem)
	if project.title != "OMEN.Delivery" || project.description != "Space" {
		t.Fatalf("project item = %#v", project)
	}
	updated, _ = projectModel.updatePicker(tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	if back := updated.(Model); back.pickerKind != pickerCategory || back.screen != screenPicker {
		t.Fatalf("esc from projects returned to kind = %v, screen = %v", back.pickerKind, back.screen)
	}

	timeOff := base
	timeOff.picker.Select(1)
	updated, _ = timeOff.updatePicker(tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter})
	timeOffModel := updated.(Model)
	if timeOffModel.pickerKind != pickerTimeOff || len(timeOffModel.picker.Items()) != 2 {
		t.Fatalf("time off picker kind = %v, items = %#v", timeOffModel.pickerKind, timeOffModel.picker.Items())
	}
	visible := timeOffModel.picker.Items()
	if visible[0].(pickerItem).title != "Vacation" || visible[1].(pickerItem).title != "FTO - No Approval Required" {
		t.Fatalf("visible time off options = %#v", visible)
	}
	updated, _ = timeOffModel.updatePicker(tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	if back := updated.(Model); back.pickerKind != pickerCategory || back.screen != screenPicker {
		t.Fatalf("esc from time off returned to kind = %v, screen = %v", back.pickerKind, back.screen)
	}
	timeOffModel.picker.Select(0)
	updated, _ = timeOffModel.updatePicker(tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter})
	form := updated.(Model)
	if form.screen != screenForm || form.draft.kind != timeOffEntry || form.draft.timeOffTypeID != "vacation" {
		t.Fatalf("time off form screen = %v, draft = %#v", form.screen, form.draft)
	}
	view := form.View()
	if !strings.Contains(view, "New time off entry") || !strings.Contains(view, "Vacation") || strings.Contains(view, "Client") {
		t.Fatalf("unexpected time off form:\n%s", view)
	}
	updated, _ = form.updateForm(tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	if back := updated.(Model); back.pickerKind != pickerTimeOff || back.screen != screenPicker {
		t.Fatalf("esc from time off form returned to kind = %v, screen = %v", back.pickerKind, back.screen)
	}
}

func TestProjectEntryEscapeMovesBackOnePage(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)
	model := NewAt(nil, func() time.Time { return date })
	model.jobs = []clicktime.Job{{ID: "job-1", Name: "OMEN.Delivery"}}
	model.availableTasks = []clicktime.Task{{ID: "task-1", Name: "Integration"}}
	model.draft = draft{
		kind: projectEntry, date: date.Format(time.DateOnly),
		jobID: "job-1", jobName: "OMEN.Delivery", taskID: "task-1", taskName: "Integration",
	}
	model.openForm()

	updated, _ := model.updateForm(tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	tasks := updated.(Model)
	if tasks.pickerKind != pickerTask || tasks.screen != screenPicker {
		t.Fatalf("esc from form returned to kind = %v, screen = %v", tasks.pickerKind, tasks.screen)
	}
	updated, _ = tasks.updatePicker(tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	projects := updated.(Model)
	if projects.pickerKind != pickerJob || projects.screen != screenPicker {
		t.Fatalf("esc from tasks returned to kind = %v, screen = %v", projects.pickerKind, projects.screen)
	}
	updated, _ = projects.updatePicker(tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	category := updated.(Model)
	if category.pickerKind != pickerCategory || category.screen != screenPicker {
		t.Fatalf("esc from projects returned to kind = %v, screen = %v", category.pickerKind, category.screen)
	}
	updated, _ = category.updatePicker(tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	if dashboard := updated.(Model); dashboard.screen != screenDashboard {
		t.Fatalf("esc from category returned to screen = %v", dashboard.screen)
	}
}

func TestTimeOffRowsAndTotals(t *testing.T) {
	t.Parallel()
	week := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	model := Model{
		weekStart:    week,
		timeOffTypes: []clicktime.TimeOffType{{ID: "vacation", Name: "Vacation"}},
		timeOffEntries: []clicktime.TimeOffEntry{{
			ID: "off-1", Date: "2026-07-29", Hours: 8, TimeOffTypeID: "vacation", Comment: "Summer break",
		}},
	}
	rows := model.timesheetRows()
	if len(rows) != 1 || rows[0].project != "Time Off" || rows[0].task != "Vacation" || rows[0].hours[2] != 8 {
		t.Fatalf("timesheetRows() = %#v", rows)
	}
	model.dayCursor = 2
	entries := model.selectedEntries()
	if len(entries) != 1 || entries[0].kind != timeOffEntry || entries[0].timeOff.Key() != "off-1" {
		t.Fatalf("selectedEntries() = %#v", entries)
	}
	if model.weekTotal() != 8 || model.totalForDate(week.AddDate(0, 0, 2)) != 8 {
		t.Fatalf("time off totals: week=%v day=%v", model.weekTotal(), model.totalForDate(week.AddDate(0, 0, 2)))
	}
}

func TestDashboardNavigationKeys(t *testing.T) {
	t.Parallel()
	week := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	base := Model{
		weekStart: week,
		dayCursor: 2,
		entries: []clicktime.TimeEntry{
			{ID: "1", Date: "2026-07-29", Hours: 1, JobID: "job-1", TaskID: "task-1"},
			{ID: "2", Date: "2026-07-29", Hours: 1, JobID: "job-2", TaskID: "task-2"},
		},
	}

	updated, _ := base.updateDashboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if got := updated.(Model).dayCursor; got != 3 {
		t.Fatalf("l dayCursor = %d, want 3", got)
	}
	updated, _ = base.updateDashboard(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(Model).dayCursor; got != 3 {
		t.Fatalf("right dayCursor = %d, want 3", got)
	}
	updated, _ = base.updateDashboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if got := updated.(Model).dayCursor; got != 1 {
		t.Fatalf("h dayCursor = %d, want 1", got)
	}
	updated, _ = base.updateDashboard(tea.KeyMsg{Type: tea.KeyLeft})
	if got := updated.(Model).dayCursor; got != 1 {
		t.Fatalf("left dayCursor = %d, want 1", got)
	}
	updated, _ = base.updateDashboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := updated.(Model).cursor; got != 1 {
		t.Fatalf("j cursor = %d, want 1", got)
	}
	updated, _ = updated.(Model).updateDashboard(tea.KeyMsg{Type: tea.KeyUp})
	if got := updated.(Model).cursor; got != 0 {
		t.Fatalf("up cursor = %d, want 0", got)
	}
	updated, _ = base.updateDashboard(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(Model).cursor; got != 1 {
		t.Fatalf("down cursor = %d, want 1", got)
	}
	updated, _ = base.updateDashboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	previous := updated.(Model)
	if got := previous.weekStart.Format(time.DateOnly); got != "2026-07-20" || previous.screen != screenLoading {
		t.Fatalf("[ week = %s, screen = %v", got, previous.screen)
	}
	updated, _ = base.updateDashboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if got := updated.(Model).weekStart.Format(time.DateOnly); got != "2026-08-03" {
		t.Fatalf("] week = %s, want 2026-08-03", got)
	}
}

func TestEditEmptyProjectCellStartsNewEntryForSelectedRow(t *testing.T) {
	t.Parallel()
	week := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	model := NewAt(nil, func() time.Time { return week })
	model.screen = screenDashboard
	model.weekStart = week
	model.dayCursor = 1
	model.clients = []clicktime.ClientResource{{ID: "client-1", Name: "Space"}}
	model.jobs = []clicktime.Job{{ID: "job-1", ClientID: "client-1", Name: "Apollo"}}
	model.tasks = []clicktime.Task{{ID: "task-1", Name: "Labor"}}
	model.entries = []clicktime.TimeEntry{
		{ID: "entry-1", Date: "2026-07-27", Hours: 2, JobID: "job-1", TaskID: "task-1"},
	}

	updated, _ := model.updateDashboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	form := updated.(Model)
	if form.screen != screenForm {
		t.Fatalf("screen = %v, want %v", form.screen, screenForm)
	}
	if form.draft.kind != projectEntry || form.draft.date != "2026-07-28" || form.draft.jobID != "job-1" || form.draft.taskID != "task-1" {
		t.Fatalf("draft = %#v", form.draft)
	}
	if form.draft.clientName != "Space" || form.draft.jobName != "Apollo" || form.draft.taskName != "Labor" {
		t.Fatalf("draft labels = client %q, job %q, task %q", form.draft.clientName, form.draft.jobName, form.draft.taskName)
	}
	if form.hoursInput.Value() != "" || form.notesInput.Value() != "" || form.formFocus != 0 {
		t.Fatalf("form inputs = hours %q, notes %q, focus %d", form.hoursInput.Value(), form.notesInput.Value(), form.formFocus)
	}

	updated, _ = form.updateForm(tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	if dashboard := updated.(Model); dashboard.screen != screenDashboard {
		t.Fatalf("esc returned to screen = %v, want %v", dashboard.screen, screenDashboard)
	}
}

func TestEntryFormDateIsReadOnly(t *testing.T) {
	t.Parallel()
	model := NewAt(nil, func() time.Time {
		return time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)
	})
	model.width, model.height = 80, 30
	model.draft = draft{
		date: "2026-07-29", clientName: "Space", jobName: "OMEN.Delivery", taskName: "Integration",
	}
	model.openForm()
	view := model.View()
	if !strings.Contains(view, "Wed, Jul 29, 2026") {
		t.Fatalf("form does not display the selected date:\n%s", view)
	}
	if strings.Contains(view, "YYYY-MM-DD") {
		t.Fatalf("form still contains an editable date input:\n%s", view)
	}
	if model.notesInput.Prompt != "" || strings.Contains(view, "┃") {
		t.Fatalf("form contains the textarea prompt artifact:\n%s", view)
	}

	model.hoursInput.SetValue("8")
	updated, _ := model.prepareReview()
	review := updated.(Model)
	if review.draft.date != "2026-07-29" || review.screen != screenReview {
		t.Fatalf("review date = %q, screen = %v", review.draft.date, review.screen)
	}
}

func TestEnterOnEmptyNotesReviewsEntry(t *testing.T) {
	t.Parallel()
	model := NewAt(nil, func() time.Time {
		return time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)
	})
	model.draft = draft{
		date: "2026-07-29", clientName: "Space", jobName: "OMEN.Delivery", taskName: "Integration",
	}
	model.openForm()
	model.hoursInput.SetValue("8")
	model.setFormFocus(1)

	updated, _ := model.updateForm(tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter})
	review := updated.(Model)
	if review.screen != screenReview {
		t.Fatalf("screen = %v, want %v", review.screen, screenReview)
	}
	if review.draft.hours != 8 || review.draft.comment != "" {
		t.Fatalf("draft = %#v", review.draft)
	}
}

func TestEnterOnPopulatedNotesInsertsNewline(t *testing.T) {
	t.Parallel()
	model := NewAt(nil, func() time.Time {
		return time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)
	})
	model.draft = draft{
		date: "2026-07-29", clientName: "Space", jobName: "OMEN.Delivery", taskName: "Integration",
	}
	model.openForm()
	model.hoursInput.SetValue("8")
	model.notesInput.SetValue("Already started")
	model.setFormFocus(1)

	updated, _ := model.updateForm(tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter})
	form := updated.(Model)
	if form.screen != screenForm {
		t.Fatalf("screen = %v, want %v", form.screen, screenForm)
	}
	if !strings.Contains(form.notesInput.Value(), "\n") {
		t.Fatalf("notes value = %q, want newline", form.notesInput.Value())
	}
}

func TestResolveTaskIDsFromCatalog(t *testing.T) {
	t.Parallel()
	catalog := []clicktime.Task{
		{ID: "task-1", Name: "Application Integration", IsActive: true},
		{ID: "task-2", Name: "Review", IsActive: true},
	}
	resolved := resolveTasks([]clicktime.Task{{ID: "task-1"}}, catalog)
	if len(resolved) != 1 || resolved[0].Label() != "Application Integration" || !resolved[0].IsActive {
		t.Fatalf("resolveTasks() = %#v", resolved)
	}

	merged := mergeTasks(catalog, []clicktime.Task{{ID: "task-1"}})
	for _, task := range merged {
		if task.ID == "task-1" && task.Label() != "Application Integration" {
			t.Fatalf("mergeTasks() discarded task metadata: %#v", task)
		}
	}
}

func TestDashboardIsCenteredAndTabular(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	model := Model{
		now:   func() time.Time { return now },
		width: 120, height: 30,
		screen:    screenDashboard,
		weekStart: startOfWeek(now),
		me:        clicktime.Me{Name: "Test User"},
		entries: []clicktime.TimeEntry{{
			ID: "entry-1", Date: "2026-07-28", Hours: 8,
			JobID: "job-1", TaskID: "task-1",
		}},
		jobs:  []clicktime.Job{{ID: "job-1", Name: "Apollo"}},
		tasks: []clicktime.Task{{ID: "task-1", Name: "Labor"}},
	}
	view := model.View()
	if got := lipgloss.Width(view); got != 120 {
		t.Fatalf("view width = %d, want 120", got)
	}
	if got := lipgloss.Height(view); got != 30 {
		t.Fatalf("view height = %d, want 30", got)
	}
	if !strings.Contains(view, "Project") || !strings.Contains(view, "Task") || !strings.Contains(view, "Tue 28") {
		t.Fatalf("dashboard does not contain expected table headers:\n%s", view)
	}
	line := 0
	for index, value := range strings.Split(view, "\n") {
		if strings.Contains(value, "tuitime") {
			line = index
			break
		}
	}
	if line == 0 {
		t.Fatalf("dashboard was not vertically centered:\n%s", view)
	}

	detailColumn, helpColumn := -1, -1
	for _, value := range strings.Split(view, "\n") {
		if index := strings.Index(value, "Mon, Jul 27"); index >= 0 {
			detailColumn = lipgloss.Width(value[:index])
		}
		if strings.Contains(value, "q quit") {
			if index := strings.Index(value, "hl"); index >= 0 {
				helpColumn = lipgloss.Width(value[:index])
			}
		}
	}
	if detailColumn < 0 || helpColumn < 0 || detailColumn != helpColumn {
		t.Fatalf("detail column = %d, help column = %d; want matching alignment:\n%s", detailColumn, helpColumn, view)
	}
}

func TestDashboardMarksTimesheetEnd(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	model := Model{
		now:   func() time.Time { return now },
		width: 120, height: 30,
		screen:    screenDashboard,
		weekStart: time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC),
		me:        clicktime.Me{Name: "Test User"},
		entries: []clicktime.TimeEntry{{
			ID: "entry-1", Date: "2026-07-15", Hours: 8,
			JobID: "job-1", TaskID: "task-1",
		}},
		jobs:  []clicktime.Job{{ID: "job-1", Name: "Apollo"}},
		tasks: []clicktime.Task{{ID: "task-1", Name: "Labor"}},
	}
	view := model.View()
	if !strings.Contains(view, "Wed 15※") {
		t.Fatalf("dashboard does not mark today when it is a timesheet end:\n%s", view)
	}
	if !strings.Contains(view, "+ timesheet end") {
		t.Fatalf("dashboard does not explain timesheet end marker:\n%s", view)
	}
	if !strings.Contains(view, "※ today and timesheet end") {
		t.Fatalf("dashboard does not explain combined marker:\n%s", view)
	}
	legend := strings.Index(view, "* today")
	help := strings.Index(view, "q quit")
	if legend < 0 || help < 0 || legend < help {
		t.Fatalf("dashboard marker legend should appear below the controls:\n%s", view)
	}

	model.weekStart = time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	view = model.View()
	if !strings.Contains(view, "Fri 31+") {
		t.Fatalf("dashboard does not mark month end as timesheet end:\n%s", view)
	}
}
