package infinite

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/resnostyle/infinite-mqtt/internal/lib/campus"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// StudentSlug builds a stable HA-safe slug from first name + personID.
func StudentSlug(s campus.Student) string {
	name := strings.ToLower(strings.TrimSpace(s.FirstName))
	name = nonSlug.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		name = "student"
	}
	return fmt.Sprintf("%s_%d", name, s.PersonID)
}

// CourseSlug builds a HA-safe course object id fragment.
func CourseSlug(courseName string, sectionID int) string {
	name := strings.ToLower(strings.TrimSpace(courseName))
	var b strings.Builder
	prevUnderscore := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	slug := strings.Trim(b.String(), "_")
	if slug == "" {
		slug = "course"
	}
	if len(slug) > 40 {
		slug = slug[:40]
		slug = strings.TrimRight(slug, "_")
	}
	if sectionID > 0 {
		return fmt.Sprintf("%s_%d", slug, sectionID)
	}
	return slug
}

// StudentSnapshot is published MQTT payload for one student.
type StudentSnapshot struct {
	Slug        string               `json:"slug"`
	PersonID    int                  `json:"person_id"`
	FirstName   string               `json:"first_name"`
	LastName    string               `json:"last_name"`
	Grade       string               `json:"grade,omitempty"`
	School      string               `json:"school,omitempty"`
	Grades      []campus.CourseGrade `json:"grades"`
	Assignments []AssignmentOut      `json:"assignments"`
	Missing     []AssignmentOut      `json:"missing"`
	Upcoming    []AssignmentOut      `json:"upcoming"`
	Summary     StudentSummary       `json:"summary"`
}

// StudentSummary aggregates counts for discovery sensors.
type StudentSummary struct {
	CourseCount     int `json:"course_count"`
	AssignmentCount int `json:"assignment_count"`
	MissingCount    int `json:"missing_count"`
	UpcomingCount   int `json:"upcoming_count"`
	GradedCount     int `json:"graded_count"`
	TurnedInCount   int `json:"turned_in_count"`
}

// AssignmentOut is a trimmed assignment for MQTT.
type AssignmentOut struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	CourseName      string  `json:"course_name"`
	DueDate         string  `json:"due_date,omitempty"`
	AssignedDate    string  `json:"assigned_date,omitempty"`
	Score           string  `json:"score,omitempty"`
	ScorePercentage string  `json:"score_percentage,omitempty"`
	TotalPoints     float64 `json:"total_points,omitempty"`
	Missing         bool    `json:"missing"`
	Late            bool    `json:"late"`
	TurnedIn        bool    `json:"turned_in"`
	Incomplete      bool    `json:"incomplete"`
	Dropped         bool    `json:"dropped"`
	NotGraded       bool    `json:"not_graded"`
}

// StatusPayload is the top-level connection status topic.
type StatusPayload struct {
	Connected    bool   `json:"connected"`
	LastUpdated  string `json:"last_updated"`
	StudentCount int    `json:"student_count"`
	Error        string `json:"error,omitempty"`
}

// StudentListItem is a compact student entry for the students topic.
type StudentListItem struct {
	Slug      string `json:"slug"`
	PersonID  int    `json:"person_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Grade     string `json:"grade,omitempty"`
	School    string `json:"school,omitempty"`
}

// BuildSnapshot aggregates grades and assignments for one student.
func BuildSnapshot(student campus.Student, grades []campus.CourseGrade, assignments []campus.Assignment, upcomingDays int, now time.Time) StudentSnapshot {
	slug := StudentSlug(student)
	gradeLevel, school := enrollmentMeta(student)

	outs := make([]AssignmentOut, 0, len(assignments))
	var missing, upcoming []AssignmentOut
	graded, turnedIn := 0, 0
	horizon := now.AddDate(0, 0, upcomingDays)

	for _, a := range assignments {
		out := toAssignmentOut(a)
		outs = append(outs, out)
		if a.Missing {
			missing = append(missing, out)
		}
		if a.TurnedIn {
			turnedIn++
		}
		if strings.TrimSpace(a.Score) != "" || strings.TrimSpace(a.ScorePercentage) != "" {
			graded++
		}
		if due, ok := parseDue(a.DueDate); ok {
			if !due.Before(now.Truncate(24*time.Hour)) && !due.After(horizon) && !a.Dropped {
				upcoming = append(upcoming, out)
			}
		}
	}

	return StudentSnapshot{
		Slug:        slug,
		PersonID:    student.PersonID,
		FirstName:   student.FirstName,
		LastName:    student.LastName,
		Grade:       gradeLevel,
		School:      school,
		Grades:      grades,
		Assignments: outs,
		Missing:     missing,
		Upcoming:    upcoming,
		Summary: StudentSummary{
			CourseCount:     len(grades),
			AssignmentCount: len(outs),
			MissingCount:    len(missing),
			UpcomingCount:   len(upcoming),
			GradedCount:     graded,
			TurnedInCount:   turnedIn,
		},
	}
}

func enrollmentMeta(student campus.Student) (grade, school string) {
	if len(student.Enrollments) == 0 {
		return "", ""
	}
	en := student.Enrollments[0]
	return en.Grade, en.SchoolName
}

func toAssignmentOut(a campus.Assignment) AssignmentOut {
	out := AssignmentOut{
		ID:              a.ObjectSectionID,
		Name:            a.AssignmentName,
		CourseName:      a.CourseName,
		DueDate:         a.DueDate,
		AssignedDate:    a.AssignedDate,
		Score:           a.Score,
		ScorePercentage: a.ScorePercentage,
		Missing:         a.Missing,
		Late:            a.Late,
		TurnedIn:        a.TurnedIn,
		Incomplete:      a.Incomplete,
		Dropped:         a.Dropped,
		NotGraded:       a.NotGraded,
	}
	if a.TotalPoints != nil {
		out.TotalPoints = *a.TotalPoints
	}
	return out
}

func parseDue(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.000",
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, true
		}
	}
	// Truncate timezone / millis variants.
	if i := strings.IndexAny(raw, "Z+"); i > 10 {
		if t, err := time.ParseInLocation("2006-01-02T15:04:05", raw[:19], time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// RosterKey is a stable fingerprint of students + courses for discovery republish.
func RosterKey(snapshots []StudentSnapshot) string {
	var b strings.Builder
	for _, s := range snapshots {
		b.WriteString(s.Slug)
		b.WriteByte('|')
		for _, g := range s.Grades {
			b.WriteString(CourseSlug(g.CourseName, g.SectionID))
			b.WriteByte(',')
		}
		b.WriteByte(';')
	}
	return b.String()
}
