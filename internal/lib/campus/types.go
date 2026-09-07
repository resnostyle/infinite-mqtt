package campus

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Student is a linked child on the parent portal.
type Student struct {
	PersonID            int          `json:"personID"`
	Guardian            bool         `json:"guardian"`
	FirstName           string       `json:"firstName"`
	LastName            string       `json:"lastName"`
	MiddleName          string       `json:"middleName"`
	StudentNumber       int          `json:"studentNumber"`
	HasPortalEnrollment bool         `json:"hasPortalEnrollment"`
	Enrollments         []Enrollment `json:"enrollments"`
}

// Enrollment is a school calendar enrollment.
type Enrollment struct {
	EnrollmentID      int    `json:"enrollmentID"`
	PersonID          int    `json:"personID"`
	CalendarID        int    `json:"calendarID"`
	StructureID       int    `json:"structureID"`
	Grade             string `json:"grade"`
	StructureName     string `json:"structureName"`
	CalendarName      string `json:"calendarName"`
	SchoolID          int    `json:"schoolID"`
	SchoolName        string `json:"schoolName"`
	CalendarStartDate string `json:"calendarStartDate"`
	CalendarEndDate   string `json:"calendarEndDate"`
}

// Assignment is a portal assignment listView row.
type Assignment struct {
	ObjectSectionID int      `json:"objectSectionID"`
	PersonID        int      `json:"personID"`
	AssignmentName  string   `json:"assignmentName"`
	SectionID       int      `json:"sectionID"`
	DueDate         string   `json:"dueDate"`
	AssignedDate    string   `json:"assignedDate"`
	ModifiedDate    string   `json:"modifiedDate"`
	CourseName      string   `json:"courseName"`
	Active          bool     `json:"active"`
	Score           string   `json:"score"`
	ScorePoints     string   `json:"scorePoints"`
	ScorePercentage string   `json:"scorePercentage"`
	TotalPoints     *float64 `json:"totalPoints"`
	Comments        string   `json:"comments"`
	Late            bool     `json:"late"`
	Missing         bool     `json:"missing"`
	Dropped         bool     `json:"dropped"`
	Incomplete      bool     `json:"incomplete"`
	TurnedIn        bool     `json:"turnedIn"`
	NotGraded       bool     `json:"notGraded"`
}

// RosterCourse is a roster entry used to enrich grades.
type RosterCourse struct {
	SectionID      int    `json:"sectionID"`
	CourseID       int    `json:"courseID"`
	CourseName     string `json:"courseName"`
	CourseNumber   string `json:"courseNumber"`
	SchoolName     string `json:"schoolName"`
	RoomName       string `json:"roomName"`
	TeacherDisplay string `json:"teacherDisplay"`
}

// CourseGrade is a normalized course grade for MQTT/HA.
type CourseGrade struct {
	SectionID    int     `json:"section_id"`
	CourseName   string  `json:"course_name"`
	CourseNumber string  `json:"course_number,omitempty"`
	Teacher      string  `json:"teacher,omitempty"`
	Room         string  `json:"room,omitempty"`
	TermName     string  `json:"term_name,omitempty"`
	TaskName     string  `json:"task_name,omitempty"`
	Letter       string  `json:"letter,omitempty"`
	Percent      float64 `json:"percent,omitempty"`
	Score        string  `json:"score,omitempty"`
	PersonID     int     `json:"person_id"`
}

// ParseGrades extracts CourseGrade rows from district-varying grades JSON.
func ParseGrades(raw json.RawMessage, personID int) []CourseGrade {
	var out []CourseGrade

	// Array of student grade trees.
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, item := range arr {
			out = append(out, parseGradeNode(item, personID)...)
		}
		return out
	}

	// Single object.
	out = append(out, parseGradeNode(raw, personID)...)
	return out
}

func parseGradeNode(raw json.RawMessage, personID int) []CourseGrade {
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil
	}

	if pid := intFromRaw(node["personID"]); pid != 0 {
		personID = pid
	}

	var out []CourseGrade

	if terms, ok := node["terms"]; ok {
		var termArr []json.RawMessage
		if err := json.Unmarshal(terms, &termArr); err == nil {
			for _, t := range termArr {
				out = append(out, parseTerm(t, personID)...)
			}
		}
	}

	if courses, ok := node["courses"]; ok {
		out = append(out, parseCourses(courses, personID, "")...)
	}

	if details, ok := node["details"]; ok {
		out = append(out, parseCourses(details, personID, "")...)
	}

	// Flat course-like object.
	if _, hasName := node["courseName"]; hasName {
		out = append(out, courseFromMap(node, personID, "")...)
	}

	return out
}

func parseTerm(raw json.RawMessage, personID int) []CourseGrade {
	var term map[string]json.RawMessage
	if err := json.Unmarshal(raw, &term); err != nil {
		return nil
	}
	termName := stringFromRaw(term["termName"])
	if termName == "" {
		termName = stringFromRaw(term["name"])
	}
	if courses, ok := term["courses"]; ok {
		return parseCourses(courses, personID, termName)
	}
	if sections, ok := term["sections"]; ok {
		return parseCourses(sections, personID, termName)
	}
	return nil
}

func parseCourses(raw json.RawMessage, personID int, termName string) []CourseGrade {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	var out []CourseGrade
	for _, item := range arr {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(item, &m); err != nil {
			continue
		}
		out = append(out, courseFromMap(m, personID, termName)...)
	}
	return out
}

func courseFromMap(m map[string]json.RawMessage, personID int, termName string) []CourseGrade {
	courseName := stringFromRaw(m["courseName"])
	if courseName == "" {
		courseName = stringFromRaw(m["name"])
	}
	if courseName == "" {
		return nil
	}
	base := CourseGrade{
		SectionID:    intFromRaw(m["sectionID"]),
		CourseName:   courseName,
		CourseNumber: stringFromRaw(m["courseNumber"]),
		Teacher:      stringFromRaw(m["teacherDisplay"]),
		Room:         stringFromRaw(m["roomName"]),
		TermName:     termName,
		PersonID:     personID,
	}
	if base.Teacher == "" {
		base.Teacher = stringFromRaw(m["teacherName"])
	}

	tasksRaw, ok := m["gradingTasks"]
	if !ok {
		tasksRaw, ok = m["tasks"]
	}
	if !ok {
		tasksRaw, ok = m["grades"]
	}
	if ok && tasksRaw != nil {
		var tasks []json.RawMessage
		if err := json.Unmarshal(tasksRaw, &tasks); err == nil && len(tasks) > 0 {
			var out []CourseGrade
			for _, t := range tasks {
				g := base
				applyTask(&g, t)
				if g.Letter != "" || g.Percent > 0 || g.Score != "" {
					out = append(out, g)
				}
			}
			if len(out) > 0 {
				return pickPrimaryGrades(out)
			}
		}
	}

	// Inline score fields on the course.
	g := base
	g.Letter = firstNonEmpty(stringFromRaw(m["score"]), stringFromRaw(m["progressScore"]), stringFromRaw(m["letterGrade"]))
	g.Score = stringFromRaw(m["score"])
	if p := floatFromRaw(m["percent"]); p > 0 {
		g.Percent = p
	} else if p := floatFromRaw(m["progressPercent"]); p > 0 {
		g.Percent = p
	} else if p := floatFromRaw(m["scorePercentage"]); p > 0 {
		g.Percent = p
	}
	if g.Letter != "" || g.Percent > 0 {
		return []CourseGrade{g}
	}
	// Still emit course shell so roster enrichment can attach later.
	return []CourseGrade{g}
}

func applyTask(g *CourseGrade, raw json.RawMessage) {
	var t map[string]json.RawMessage
	if err := json.Unmarshal(raw, &t); err != nil {
		return
	}
	g.TaskName = firstNonEmpty(stringFromRaw(t["taskName"]), stringFromRaw(t["name"]))
	g.Letter = firstNonEmpty(stringFromRaw(t["score"]), stringFromRaw(t["progressScore"]), stringFromRaw(t["letterGrade"]))
	g.Score = stringFromRaw(t["score"])
	if p := floatFromRaw(t["percent"]); p > 0 {
		g.Percent = p
	} else if p := floatFromRaw(t["progressPercent"]); p > 0 {
		g.Percent = p
	} else if p := floatFromRaw(t["scorePercentage"]); p > 0 {
		g.Percent = p
	}
	if tn := stringFromRaw(t["termName"]); tn != "" {
		g.TermName = tn
	}
}

// pickPrimaryGrades prefers overall / semester / progress tasks over category tasks.
func pickPrimaryGrades(grades []CourseGrade) []CourseGrade {
	priority := func(task string) int {
		t := strings.ToLower(task)
		switch {
		case strings.Contains(t, "semester"), strings.Contains(t, "final"), strings.Contains(t, "overall"):
			return 0
		case strings.Contains(t, "progress"), strings.Contains(t, "cumulative"), strings.Contains(t, "term"):
			return 1
		case task == "":
			return 2
		default:
			return 3
		}
	}
	best := grades[0]
	bestP := priority(best.TaskName)
	for _, g := range grades[1:] {
		p := priority(g.TaskName)
		if p < bestP {
			best = g
			bestP = p
		}
	}
	return []CourseGrade{best}
}

func stringFromRaw(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return ""
}

func intFromRaw(raw json.RawMessage) int {
	if raw == nil {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		v, _ := strconv.Atoi(strings.TrimSpace(s))
		return v
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int(f)
	}
	return 0
}

func floatFromRaw(raw json.RawMessage) float64 {
	if raw == nil {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
		v, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return v
		}
	}
	return 0
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// EnrichGrades fills teacher/room/course number from roster when missing.
func EnrichGrades(grades []CourseGrade, roster []RosterCourse) []CourseGrade {
	bySection := make(map[int]RosterCourse, len(roster))
	byName := make(map[string]RosterCourse, len(roster))
	for _, r := range roster {
		bySection[r.SectionID] = r
		byName[strings.ToLower(r.CourseName)] = r
	}
	for i := range grades {
		r, ok := bySection[grades[i].SectionID]
		if !ok {
			r, ok = byName[strings.ToLower(grades[i].CourseName)]
		}
		if !ok {
			continue
		}
		if grades[i].Teacher == "" {
			grades[i].Teacher = r.TeacherDisplay
		}
		if grades[i].Room == "" {
			grades[i].Room = r.RoomName
		}
		if grades[i].CourseNumber == "" {
			grades[i].CourseNumber = r.CourseNumber
		}
		if grades[i].SectionID == 0 {
			grades[i].SectionID = r.SectionID
		}
	}
	return grades
}
