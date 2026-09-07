package campus_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/resnostyle/infinite-mqtt/internal/lib/campus"
)

func TestAuthURL(t *testing.T) {
	c, err := campus.New("https://example.infinitecampus.org", "user", "pass", "mydistrict")
	if err != nil {
		t.Fatal(err)
	}
	u := c.AuthURL()
	if !strings.Contains(u, "/campus/verify.jsp?") {
		t.Fatalf("unexpected auth url: %s", u)
	}
	if !strings.Contains(u, "appName=mydistrict") {
		t.Fatalf("missing district: %s", u)
	}
	if !strings.Contains(u, "portalLoginPage=parents") {
		t.Fatalf("missing portalLoginPage: %s", u)
	}
	if !strings.Contains(u, "nonBrowser=true") {
		t.Fatalf("missing nonBrowser: %s", u)
	}
}

func TestGradePaths(t *testing.T) {
	student := campus.Student{
		PersonID: 42,
		Enrollments: []campus.Enrollment{
			{EnrollmentID: 7, StructureID: 3, CalendarID: 9},
		},
	}
	paths := campus.GradePaths(student)
	wantSub := []string{
		"/campus/resources/portal/grades",
		"personID=42",
		"studentID=42",
		"enrollmentID=7",
		"structureID=3",
		"calendarID=9",
	}
	joined := strings.Join(paths, "\n")
	for _, s := range wantSub {
		if !strings.Contains(joined, s) {
			t.Fatalf("grade paths missing %q:\n%s", s, joined)
		}
	}
}

func TestParseGradesNested(t *testing.T) {
	raw := json.RawMessage(`[{
		"personID": 10,
		"terms": [{
			"termName": "Q1",
			"courses": [{
				"sectionID": 55,
				"courseName": "Algebra I",
				"teacherDisplay": "Ms. Smith",
				"gradingTasks": [
					{"taskName": "Category Homework", "score": "B", "percent": 85},
					{"taskName": "Semester Grade", "score": "A", "percent": 92.5}
				]
			}]
		}]
	}]`)
	grades := campus.ParseGrades(raw, 0)
	if len(grades) != 1 {
		t.Fatalf("got %d grades, want 1: %+v", len(grades), grades)
	}
	g := grades[0]
	if g.Letter != "A" || g.Percent != 92.5 {
		t.Fatalf("expected semester A/92.5, got %+v", g)
	}
	if g.Teacher != "Ms. Smith" || g.CourseName != "Algebra I" {
		t.Fatalf("unexpected course fields: %+v", g)
	}
	if g.PersonID != 10 || g.SectionID != 55 {
		t.Fatalf("unexpected ids: %+v", g)
	}
}

func TestParseGradesFlat(t *testing.T) {
	raw := json.RawMessage(`{
		"courseName": "Biology",
		"sectionID": 1,
		"score": "B+",
		"percent": 88
	}`)
	grades := campus.ParseGrades(raw, 99)
	if len(grades) != 1 {
		t.Fatalf("got %d", len(grades))
	}
	if grades[0].Letter != "B+" || grades[0].PersonID != 99 {
		t.Fatalf("%+v", grades[0])
	}
}

func TestEnrichGrades(t *testing.T) {
	grades := []campus.CourseGrade{{
		SectionID:  5,
		CourseName: "Art",
	}}
	roster := []campus.RosterCourse{{
		SectionID:      5,
		CourseName:     "Art",
		TeacherDisplay: "Mr. Lee",
		RoomName:       "101",
		CourseNumber:   "ART1",
	}}
	out := campus.EnrichGrades(grades, roster)
	if out[0].Teacher != "Mr. Lee" || out[0].Room != "101" || out[0].CourseNumber != "ART1" {
		t.Fatalf("%+v", out[0])
	}
}

func TestLoginAndStudents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/campus/verify.jsp", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("username") != "parent" {
			http.Error(w, "password-error", http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "abc"})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/campus/api/portal/students", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"personID":1,"firstName":"Ada","lastName":"Lovelace","enrollments":[]}]`))
	})
	mux.HandleFunc("/campus/api/portal/assignment/listView", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"objectSectionID":9,"personID":1,"assignmentName":"HW1","courseName":"Math","dueDate":"2099-01-15","missing":true,"late":false,"turnedIn":false}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := campus.New(srv.URL, "parent", "secret", "dist")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}
	students, err := c.Students(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].FirstName != "Ada" {
		t.Fatalf("%+v", students)
	}
	asgs, err := c.Assignments(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(asgs) != 1 || !asgs[0].Missing {
		t.Fatalf("%+v", asgs)
	}
}

func TestLoginBadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("password-error"))
	}))
	defer srv.Close()
	c, err := campus.New(srv.URL, "x", "y", "z")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Login(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
