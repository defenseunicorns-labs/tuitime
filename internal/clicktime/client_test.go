package clicktime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeEntries(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Token secret" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Path; got != "/v2/Me/TimeEntries" {
			t.Errorf("path = %q", got)
		}
		if got := r.URL.Query().Get("StartDate"); got != "2026-07-27" {
			t.Errorf("StartDate = %q", got)
		}
		if got := r.URL.Query().Get("EndDate"); got != "2026-08-02" {
			t.Errorf("EndDate = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"ID":"entry-1","Date":"2026-07-27","Hours":"7.5","JobID":"job-1","TaskID":"task-1"}],"errors":[]}`))
	}))
	defer server.Close()

	client := NewWithBaseURL("secret", server.URL+"/v2", server.Client())
	start := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	entries, err := client.TimeEntries(context.Background(), start, start.AddDate(0, 0, 6))
	if err != nil {
		t.Fatalf("TimeEntries() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Key() != "entry-1" || float64(entries[0].Hours) != 7.5 {
		t.Fatalf("TimeEntries() = %#v", entries)
	}
}

func TestCreateTimeEntry(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		var input TimeEntryInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if input.Date != "2026-07-28" || input.Hours != 8 || input.JobID != "job-1" || input.TaskID != "task-1" {
			t.Errorf("body = %#v", input)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"ID":"entry-2","Date":"2026-07-28","Hours":8},"errors":[]}`))
	}))
	defer server.Close()

	client := NewWithBaseURL("secret", server.URL, server.Client())
	entry, err := client.CreateTimeEntry(context.Background(), TimeEntryInput{
		Date: "2026-07-28", Hours: 8, JobID: "job-1", TaskID: "task-1", Comment: "Work",
	})
	if err != nil {
		t.Fatalf("CreateTimeEntry() error = %v", err)
	}
	if entry.Key() != "entry-2" {
		t.Fatalf("CreateTimeEntry() = %#v", entry)
	}
}

func TestAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"Message":"Invalid Credentials"}]}`))
	}))
	defer server.Close()

	client := NewWithBaseURL("bad", server.URL, server.Client())
	_, err := client.Me(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Me() error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || apiErr.Error() != "ClickTime API returned 401: Invalid Credentials" {
		t.Fatalf("APIError = %#v (%v)", apiErr, apiErr)
	}
}

func TestTimeOff(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Me/TimeOffTypes":
			_, _ = w.Write([]byte(`{"data":[{"ID":"vacation","Name":"Vacation"}],"errors":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/Me/TimeOff":
			if r.URL.Query().Get("StartDate") != "2026-07-27" || r.URL.Query().Get("EndDate") != "2026-08-02" {
				t.Errorf("time off query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[{"ID":"off-1","Date":"2026-07-29","Hours":8,"TimeOffTypeID":"vacation"}],"errors":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Me/TimeOff":
			var input TimeOffInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("decode time off body: %v", err)
			}
			if input.TimeOffTypeID != "vacation" || input.Date != "2026-07-29" || input.Hours != 8 {
				t.Errorf("time off body = %#v", input)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"ID":"off-1"},"errors":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewWithBaseURL("secret", server.URL, server.Client())
	types, err := client.TimeOffTypes(context.Background())
	if err != nil || len(types) != 1 || types[0].Label() != "Vacation" {
		t.Fatalf("TimeOffTypes() = %#v, %v", types, err)
	}
	start := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	entries, err := client.TimeOff(context.Background(), start, start.AddDate(0, 0, 6))
	if err != nil || len(entries) != 1 || entries[0].Key() != "off-1" {
		t.Fatalf("TimeOff() = %#v, %v", entries, err)
	}
	entry, err := client.CreateTimeOff(context.Background(), TimeOffInput{
		Date: "2026-07-29", Hours: 8, TimeOffTypeID: "vacation",
	})
	if err != nil || entry.Key() != "off-1" {
		t.Fatalf("CreateTimeOff() = %#v, %v", entry, err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestTasksForJobAcceptsTaskIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/Me/Jobs/job-1/Tasks" {
			t.Errorf("path = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":["task-1",{"ID":"task-2","Name":"Review"}],"errors":[]}`))
	}))
	defer server.Close()

	client := NewWithBaseURL("secret", server.URL, server.Client())
	tasks, err := client.TasksForJob(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("TasksForJob() error = %v", err)
	}
	if len(tasks) != 2 || tasks[0].ID != "task-1" || tasks[1].Label() != "Review" {
		t.Fatalf("TasksForJob() = %#v", tasks)
	}
}

func TestTasksForJobRequiresID(t *testing.T) {
	t.Parallel()
	client := New("secret")
	if _, err := client.TasksForJob(context.Background(), ""); err == nil {
		t.Fatal("TasksForJob() error = nil")
	}
}
