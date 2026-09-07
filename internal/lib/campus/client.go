// Package campus is an Infinite Campus Parent Portal HTTP client.
package campus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// Client talks to a district Infinite Campus Parent Portal.
type Client struct {
	baseURL  string
	username string
	password string
	district string
	http     *http.Client
}

// New creates a client. baseURL is the district host (e.g. https://district.infinitecampus.org).
func New(baseURL, username, password, district string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("campus: base URL is required")
	}
	if username == "" || password == "" || district == "" {
		return nil, fmt.Errorf("campus: username, password, and district are required")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("campus: cookie jar: %w", err)
	}
	return &Client{
		baseURL:  baseURL,
		username: username,
		password: password,
		district: district,
		http: &http.Client{
			Timeout: defaultTimeout,
			Jar:     jar,
		},
	}, nil
}

// Login authenticates against the parent portal and stores session cookies.
func (c *Client) Login(ctx context.Context) error {
	u := c.authURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return fmt.Errorf("campus login: %w", err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("campus login: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("campus login read: %w", err)
	}
	text := string(body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("campus login: unexpected status %d", resp.StatusCode)
	}
	if strings.Contains(text, "password-error") {
		return fmt.Errorf("campus login: bad credentials")
	}
	return nil
}

func (c *Client) authURL() string {
	q := url.Values{}
	q.Set("nonBrowser", "true")
	q.Set("username", c.username)
	q.Set("password", c.password)
	q.Set("appName", c.district)
	q.Set("portalLoginPage", "parents")
	return c.baseURL + "/campus/verify.jsp?" + q.Encode()
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	u := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("campus GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("campus GET %s read: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("campus GET %s: status %d: %s", path, resp.StatusCode, truncate(string(body), 200))
	}
	if len(strings.TrimSpace(string(body))) == 0 || string(body) == "null" {
		return fmt.Errorf("campus GET %s: empty response", path)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("campus GET %s decode: %w", path, err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Students returns linked students for the parent account.
func (c *Client) Students(ctx context.Context) ([]Student, error) {
	var raw []Student
	if err := c.getJSON(ctx, "/campus/api/portal/students", &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// Assignments returns assignments for a student personID.
func (c *Client) Assignments(ctx context.Context, personID int) ([]Assignment, error) {
	path := fmt.Sprintf("/campus/api/portal/assignment/listView?personID=%d", personID)
	var raw []Assignment
	if err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// Roster returns course roster entries for a student.
func (c *Client) Roster(ctx context.Context, personID int) ([]RosterCourse, error) {
	path := fmt.Sprintf("/campus/resources/portal/roster?personID=%d", personID)
	var raw []RosterCourse
	if err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// Grades fetches course grades, trying several district-specific query patterns.
func (c *Client) Grades(ctx context.Context, student Student) ([]CourseGrade, error) {
	paths := gradePaths(student)
	var lastErr error
	for _, path := range paths {
		raw, err := c.fetchGradesRaw(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		grades := ParseGrades(raw, student.PersonID)
		if len(grades) > 0 {
			return grades, nil
		}
		lastErr = fmt.Errorf("campus grades: empty parse for %s", path)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("campus grades: no data for personID %d", student.PersonID)
}

func gradePaths(student Student) []string {
	paths := []string{
		"/campus/resources/portal/grades",
		fmt.Sprintf("/campus/resources/portal/grades?personID=%d", student.PersonID),
		fmt.Sprintf("/campus/resources/portal/grades?studentID=%d", student.PersonID),
	}
	for _, en := range student.Enrollments {
		paths = append(paths,
			fmt.Sprintf("/campus/resources/portal/grades?personID=%d&structureID=%d&calendarID=%d",
				student.PersonID, en.StructureID, en.CalendarID),
			fmt.Sprintf("/campus/resources/portal/grades?enrollmentID=%d", en.EnrollmentID),
		)
	}
	return paths
}

func (c *Client) fetchGradesRaw(ctx context.Context, path string) (json.RawMessage, error) {
	u := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("campus GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("campus GET %s: status %d", path, resp.StatusCode)
	}
	trim := strings.TrimSpace(string(body))
	if trim == "" || trim == "null" || trim == "[]" {
		return nil, fmt.Errorf("campus GET %s: empty", path)
	}
	return json.RawMessage(body), nil
}

// AuthURL is exported for tests.
func (c *Client) AuthURL() string {
	return c.authURL()
}

// GradePaths is exported for tests.
func GradePaths(student Student) []string {
	return gradePaths(student)
}
