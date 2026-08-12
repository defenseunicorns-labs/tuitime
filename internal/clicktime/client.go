package clicktime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.clicktime.com/v2"

// Client is a small client for the ClickTime REST API v2.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates a ClickTime client using the production API.
func New(token string) *Client {
	return NewWithBaseURL(token, DefaultBaseURL, nil)
}

// NewWithBaseURL creates a client with a custom API URL. It is primarily useful
// for tests and local development.
func NewWithBaseURL(token, baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      strings.TrimSpace(token),
		httpClient: httpClient,
	}
}

// Number accepts either a JSON number or a string containing a number. The
// ClickTime API has returned both representations for decimal fields over time.
type Number float64

func (n *Number) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*n = 0
		return nil
	}
	var value float64
	if err := json.Unmarshal(data, &value); err == nil {
		*n = Number(value)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("decode number: %w", err)
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("decode number %q: %w", text, err)
	}
	*n = Number(value)
	return nil
}

type Me struct {
	ID        string `json:"ID"`
	Name      string `json:"Name"`
	FirstName string `json:"FirstName"`
	LastName  string `json:"LastName"`
	Email     string `json:"Email"`
}

func (m Me) DisplayName() string {
	if strings.TrimSpace(m.Name) != "" {
		return m.Name
	}
	if name := strings.TrimSpace(m.FirstName + " " + m.LastName); name != "" {
		return name
	}
	return m.Email
}

type ClientResource struct {
	ID          string `json:"ID"`
	Name        string `json:"Name"`
	DisplayName string `json:"DisplayName"`
	ShortName   string `json:"ShortName"`
	IsActive    bool   `json:"IsActive"`
}

func (c ClientResource) Label() string {
	return firstNonEmpty(c.DisplayName, c.Name, c.ShortName, c.ID)
}

type Job struct {
	ID          string `json:"ID"`
	ClientID    string `json:"ClientID"`
	Name        string `json:"Name"`
	DisplayName string `json:"DisplayName"`
	JobNumber   string `json:"JobNumber"`
	IsActive    bool   `json:"IsActive"`
}

func (j Job) Label() string {
	return firstNonEmpty(j.DisplayName, j.Name, j.JobNumber, j.ID)
}

type Task struct {
	ID          string `json:"ID"`
	Name        string `json:"Name"`
	DisplayName string `json:"DisplayName"`
	Code        string `json:"Code"`
	IsActive    bool   `json:"IsActive"`
}

// UnmarshalJSON accepts both full task objects and task ID strings. ClickTime's
// /Me/Jobs/{id}/Tasks endpoint returns IDs for some account configurations,
// while /Me/Tasks returns full objects.
func (t *Task) UnmarshalJSON(data []byte) error {
	var id string
	if err := json.Unmarshal(data, &id); err == nil {
		*t = Task{ID: id}
		return nil
	}
	type taskAlias Task
	var value taskAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*t = Task(value)
	return nil
}

func (t Task) Label() string {
	return firstNonEmpty(t.DisplayName, t.Name, t.Code, t.ID)
}

type TimeOffType struct {
	ID            string `json:"ID"`
	TimeOffTypeID string `json:"TimeOffTypeID"`
	Name          string `json:"Name"`
	DisplayName   string `json:"DisplayName"`
	IsActive      bool   `json:"IsActive"`
}

func (t TimeOffType) Key() string {
	return firstNonEmpty(t.ID, t.TimeOffTypeID)
}

func (t TimeOffType) Label() string {
	return firstNonEmpty(t.DisplayName, t.Name, t.Key())
}

type TimeOffEntry struct {
	ID             string `json:"ID"`
	TimeOffEntryID string `json:"TimeOffEntryID"`
	Date           string `json:"Date"`
	Hours          Number `json:"Hours"`
	Notes          string `json:"Notes"`
	TimeOffTypeID  string `json:"TimeOffTypeID"`
}

func (e TimeOffEntry) Key() string {
	return firstNonEmpty(e.ID, e.TimeOffEntryID)
}

type TimeEntry struct {
	ID          string `json:"ID"`
	TimeEntryID string `json:"TimeEntryID"`
	Date        string `json:"Date"`
	Hours       Number `json:"Hours"`
	Comment     string `json:"Comment"`
	JobID       string `json:"JobID"`
	TaskID      string `json:"TaskID"`
	UserID      string `json:"UserID"`
	IsBillable  bool   `json:"IsBillable"`
}

func (e TimeEntry) Key() string {
	return firstNonEmpty(e.ID, e.TimeEntryID)
}

type Timesheet struct {
	ID               string `json:"ID"`
	StartDate        string `json:"StartDate"`
	EndDate          string `json:"EndDate"`
	Status           string `json:"Status"`
	HasBeenSubmitted bool   `json:"HasBeenSubmitted"`
}

// TimeEntryInput is the standard ClickTime time-entry payload. ClickTime calls
// projects "Jobs" in its REST API.
type TimeEntryInput struct {
	Date    string  `json:"Date"`
	Hours   float64 `json:"Hours"`
	JobID   string  `json:"JobID"`
	TaskID  string  `json:"TaskID"`
	Comment string  `json:"Comment"`
}

type TimeOffInput struct {
	Date          string  `json:"Date"`
	Hours         float64 `json:"Hours"`
	TimeOffTypeID string  `json:"TimeOffTypeID"`
	Notes         string  `json:"Notes"`
}

type TimeOffUpdateInput struct {
	Hours         float64 `json:"Hours"`
	TimeOffTypeID string  `json:"TimeOffTypeID"`
	Notes         string  `json:"Notes"`
}

type APIError struct {
	StatusCode int
	Messages   []string
}

func (e *APIError) Error() string {
	message := strings.Join(e.Messages, "; ")
	if message == "" {
		message = http.StatusText(e.StatusCode)
		if e.StatusCode >= 200 && e.StatusCode < 300 {
			message = "unknown error"
		}
	}
	switch {
	case e.StatusCode == 0:
		return message
	case e.StatusCode >= 200 && e.StatusCode < 300:
		return "ClickTime API reported an error: " + message
	default:
		return fmt.Sprintf("ClickTime API returned %d: %s", e.StatusCode, message)
	}
}

type responseEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []apiMessage    `json:"errors"`
	Meta   json.RawMessage `json:"meta"`
	Page   responsePage    `json:"page"`
}

type responseInfo struct {
	Meta json.RawMessage
	Page responsePage
}

type responsePage struct {
	Count  *int              `json:"count"`
	Limit  *int              `json:"limit"`
	Offset *int              `json:"offset"`
	Links  responsePageLinks `json:"links"`
}

type responsePageLinks struct {
	Next string `json:"next"`
}

type apiMessage struct {
	Field         string   `json:"Field"`
	Message       string   `json:"Message"`
	MessageDetail []string `json:"MessageDetail"`
	Detail        string   `json:"Detail"`
	Title         string   `json:"Title"`
}

func (e responseEnvelope) messages() []string {
	messages := make([]string, 0, len(e.Errors))
	for _, item := range e.Errors {
		message := firstNonEmpty(item.Message, item.Detail, item.Title)
		details := nonEmptyStrings(item.MessageDetail)
		if message == "" && len(details) > 0 {
			message, details = details[0], details[1:]
		}
		if field := strings.TrimSpace(item.Field); field != "" {
			if message == "" {
				message = field
			} else {
				message = field + ": " + message
			}
		}
		if len(details) > 0 {
			message += " (" + strings.Join(details, "; ") + ")"
		}
		if message != "" {
			messages = append(messages, message)
		}
	}
	return messages
}

func (c *Client) Me(ctx context.Context) (Me, error) {
	var result Me
	err := c.request(ctx, http.MethodGet, "/Me", nil, nil, &result)
	return result, err
}

func (c *Client) Clients(ctx context.Context) ([]ClientResource, error) {
	query := url.Values{"IsActive": {"true"}, "limit": {"1000"}}
	return requestAll[ClientResource](ctx, c, "/Me/Clients", query)
}

func (c *Client) Jobs(ctx context.Context) ([]Job, error) {
	query := url.Values{"IsActive": {"true"}, "limit": {"1000"}}
	return requestAll[Job](ctx, c, "/Me/Jobs", query)
}

func (c *Client) Tasks(ctx context.Context) ([]Task, error) {
	query := url.Values{"IsActive": {"true"}, "limit": {"1000"}}
	return requestAll[Task](ctx, c, "/Me/Tasks", query)
}

func (c *Client) TimeOffTypes(ctx context.Context) ([]TimeOffType, error) {
	query := url.Values{"IsActive": {"true"}, "limit": {"1000"}}
	return requestAll[TimeOffType](ctx, c, "/Me/TimeOffTypes", query)
}

func (c *Client) TasksForJob(ctx context.Context, jobID string) ([]Task, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, errors.New("job ID is required")
	}
	var result []Task
	query := url.Values{"IsActive": {"true"}}
	path := "/Me/Jobs/" + url.PathEscape(jobID) + "/Tasks"
	err := c.request(ctx, http.MethodGet, path, query, nil, &result)
	return result, err
}

func (c *Client) TimeEntries(ctx context.Context, start, end time.Time) ([]TimeEntry, error) {
	query := url.Values{
		"StartDate": {start.Format(time.DateOnly)},
		"EndDate":   {end.Format(time.DateOnly)},
		"limit":     {"100"},
	}
	return requestAll[TimeEntry](ctx, c, "/Me/TimeEntries", query)
}

func (c *Client) TimeOff(ctx context.Context, start, end time.Time) ([]TimeOffEntry, error) {
	query := url.Values{
		"FromDate": {start.Format(time.DateOnly)},
		"ToDate":   {end.Format(time.DateOnly)},
		"limit":    {"1000"},
	}
	return requestAll[TimeOffEntry](ctx, c, "/Me/TimeOff", query)
}

func (c *Client) Timesheets(ctx context.Context, start, end time.Time) ([]Timesheet, error) {
	query := url.Values{
		"FromDate": {start.Format(time.DateOnly)},
		"ToDate":   {end.Format(time.DateOnly)},
		"limit":    {"1000"},
	}
	return requestAll[Timesheet](ctx, c, "/Me/Timesheets", query)
}

func (c *Client) CreateTimeEntry(ctx context.Context, input TimeEntryInput) (TimeEntry, error) {
	var result TimeEntry
	err := c.request(ctx, http.MethodPost, "/Me/TimeEntries", nil, input, &result)
	return result, err
}

func (c *Client) UpdateTimeEntry(ctx context.Context, entryID string, input TimeEntryInput) (TimeEntry, error) {
	if strings.TrimSpace(entryID) == "" {
		return TimeEntry{}, errors.New("time entry ID is required")
	}
	var result TimeEntry
	path := "/Me/TimeEntries/" + url.PathEscape(entryID)
	err := c.request(ctx, http.MethodPatch, path, nil, input, &result)
	return result, err
}

func (c *Client) CreateTimeOff(ctx context.Context, input TimeOffInput) (TimeOffEntry, error) {
	var result TimeOffEntry
	err := c.request(ctx, http.MethodPost, "/Me/TimeOff", nil, input, &result)
	return result, err
}

func (c *Client) UpdateTimeOff(ctx context.Context, entryID string, input TimeOffUpdateInput) (TimeOffEntry, error) {
	if strings.TrimSpace(entryID) == "" {
		return TimeOffEntry{}, errors.New("time off entry ID is required")
	}
	var result TimeOffEntry
	path := "/Me/TimeOff/" + url.PathEscape(entryID)
	err := c.request(ctx, http.MethodPatch, path, nil, input, &result)
	return result, err
}

func requestAll[T any](ctx context.Context, client *Client, path string, query url.Values) ([]T, error) {
	pageQuery := make(url.Values, len(query)+1)
	for key, values := range query {
		pageQuery[key] = append([]string(nil), values...)
	}

	offset, err := strconv.Atoi(pageQuery.Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}
	pageQuery.Set("offset", strconv.Itoa(offset))

	var result []T
	for {
		var items []T
		info, err := client.requestWithInfo(ctx, http.MethodGet, path, pageQuery, nil, &items)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(items) == 0 {
			return result, nil
		}

		currentOffset := offset
		if info.Page.Offset != nil {
			currentOffset = *info.Page.Offset
		}
		step := len(items)
		if info.Page.Limit != nil && *info.Page.Limit > 0 {
			step = *info.Page.Limit
		}
		nextOffset := currentOffset + step
		if !info.Page.hasNext(nextOffset) {
			return result, nil
		}
		if nextOffset <= offset {
			return nil, errors.New("ClickTime returned invalid pagination metadata")
		}
		offset = nextOffset
		pageQuery.Set("offset", strconv.Itoa(offset))
	}
}

func (p responsePage) hasNext(nextOffset int) bool {
	if p.Count != nil {
		return nextOffset < *p.Count
	}
	return strings.TrimSpace(p.Links.Next) != ""
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body, result any) error {
	_, err := c.requestWithMeta(ctx, method, path, query, body, result)
	return err
}

func (c *Client) requestWithMeta(ctx context.Context, method, path string, query url.Values, body, result any) (json.RawMessage, error) {
	info, err := c.requestWithInfo(ctx, method, path, query, body, result)
	return info.Meta, err
}

func (c *Client) requestWithInfo(ctx context.Context, method, path string, query url.Values, body, result any) (responseInfo, error) {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return responseInfo{}, fmt.Errorf("encode ClickTime request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return responseInfo{}, fmt.Errorf("create ClickTime request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return responseInfo{}, fmt.Errorf("contact ClickTime: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return responseInfo{}, fmt.Errorf("read ClickTime response: %w", err)
	}
	if resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(payload)) == 0 {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return responseInfo{}, nil
		}
		return responseInfo{}, &APIError{StatusCode: resp.StatusCode}
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return responseInfo{}, &APIError{StatusCode: resp.StatusCode, Messages: []string{strings.TrimSpace(string(payload))}}
		}
		return responseInfo{}, fmt.Errorf("decode ClickTime response: %w", err)
	}
	info := responseInfo{Meta: envelope.Meta, Page: envelope.Page}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || len(envelope.Errors) > 0 {
		return info, &APIError{StatusCode: resp.StatusCode, Messages: envelope.messages()}
	}
	if result == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return info, nil
	}
	if err := json.Unmarshal(envelope.Data, result); err != nil {
		return info, fmt.Errorf("decode ClickTime data: %w", err)
	}
	return info, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
