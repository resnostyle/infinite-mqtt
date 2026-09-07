package infinite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/resnostyle/infinite-mqtt/internal/lib/campus"
	"github.com/resnostyle/mqttkit/env"
	"github.com/resnostyle/mqttkit/mqttpub"
)

func TestStudentSlug(t *testing.T) {
	got := StudentSlug(campus.Student{FirstName: "Mary-Jane", PersonID: 123})
	if got != "mary_jane_123" {
		t.Fatalf("got %q", got)
	}
}

func TestCourseSlug(t *testing.T) {
	got := CourseSlug("AP World History!", 77)
	if got != "ap_world_history_77" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildSnapshotMissingUpcoming(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.Local)
	student := campus.Student{
		PersonID:  1,
		FirstName: "Ada",
		LastName:  "L",
		Enrollments: []campus.Enrollment{
			{Grade: "9", SchoolName: "HS"},
		},
	}
	grades := []campus.CourseGrade{{CourseName: "Math", Letter: "A", SectionID: 1}}
	asgs := []campus.Assignment{
		{ObjectSectionID: 1, AssignmentName: "Missing HW", CourseName: "Math", Missing: true, DueDate: "2026-09-01"},
		{ObjectSectionID: 2, AssignmentName: "Due Soon", CourseName: "Math", DueDate: "2026-09-10"},
		{ObjectSectionID: 3, AssignmentName: "Far Away", CourseName: "Math", DueDate: "2026-12-01"},
		{ObjectSectionID: 4, AssignmentName: "Graded", CourseName: "Math", Score: "10/10", ScorePercentage: "100", TurnedIn: true, DueDate: "2026-09-05"},
	}
	snap := BuildSnapshot(student, grades, asgs, 7, now)
	if snap.Slug != "ada_1" {
		t.Fatalf("slug %q", snap.Slug)
	}
	if snap.Summary.MissingCount != 1 {
		t.Fatalf("missing %d", snap.Summary.MissingCount)
	}
	if snap.Summary.UpcomingCount != 1 {
		t.Fatalf("upcoming %d: %+v", snap.Summary.UpcomingCount, snap.Upcoming)
	}
	if snap.Upcoming[0].Name != "Due Soon" {
		t.Fatalf("%+v", snap.Upcoming)
	}
	if snap.Summary.GradedCount != 1 || snap.Summary.TurnedInCount != 1 {
		t.Fatalf("%+v", snap.Summary)
	}
}

func TestBuildDiscoveryConfigs(t *testing.T) {
	snap := StudentSnapshot{
		Slug:      "ada_1",
		FirstName: "Ada",
		Grades: []campus.CourseGrade{
			{CourseName: "Math", SectionID: 9, Letter: "A"},
		},
	}
	configs := BuildDiscoveryConfigs("home/infinite", []StudentSnapshot{snap})
	if len(configs) < 5 {
		t.Fatalf("expected status+student sensors, got %d", len(configs))
	}
	foundCourse := false
	foundMissing := false
	for _, c := range configs {
		if c.ObjectID == "infinite_campus_ada_1_math_9" {
			foundCourse = true
		}
		if c.ObjectID == "infinite_campus_ada_1_missing_assignments" {
			foundMissing = true
		}
	}
	if !foundCourse || !foundMissing {
		t.Fatalf("missing expected discovery ids")
	}
}

type mockSink struct {
	topics []string
}

func (m *mockSink) Publish(suffix string, _ any, _ bool) error {
	m.topics = append(m.topics, suffix)
	return nil
}
func (m *mockSink) PublishQuiet(suffix string, payload any, retain bool) error {
	return m.Publish(suffix, payload, retain)
}
func (m *mockSink) PublishRaw(topic string, _ any, _ bool) error {
	m.topics = append(m.topics, topic)
	return nil
}
func (m *mockSink) PublishDiscovery(_ []mqttpub.Config, discoveryPrefix string) error {
	m.topics = append(m.topics, "discovery:"+discoveryPrefix)
	return nil
}

type mockFetcher struct{}

func (mockFetcher) Login(context.Context) error { return nil }
func (mockFetcher) Students(context.Context) ([]campus.Student, error) {
	return []campus.Student{{PersonID: 1, FirstName: "Ada", LastName: "L"}}, nil
}
func (mockFetcher) Grades(context.Context, campus.Student) ([]campus.CourseGrade, error) {
	return []campus.CourseGrade{{CourseName: "Math", SectionID: 1, Letter: "A", Percent: 95}}, nil
}
func (mockFetcher) Assignments(context.Context, int) ([]campus.Assignment, error) {
	return []campus.Assignment{{ObjectSectionID: 1, AssignmentName: "HW", CourseName: "Math", Missing: true}}, nil
}
func (mockFetcher) Roster(context.Context, int) ([]campus.RosterCourse, error) {
	return []campus.RosterCourse{{SectionID: 1, CourseName: "Math", TeacherDisplay: "T"}}, nil
}

func TestFetchAndPublish(t *testing.T) {
	sink := &mockSink{}
	pub := NewPublisher()
	settings := Settings{
		MQTT: env.MQTT{
			MQTTDiscoveryEnabled: true,
			MQTTTopicPrefix:      "home/infinite",
			MQTTDiscoveryPrefix:  "homeassistant",
		},
		UpcomingDays: 7,
	}

	if err := pub.FetchAndPublish(context.Background(), settings, mockFetcher{}, sink); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(sink.topics, "\n")
	for _, want := range []string{"status", "students", "ada_1/grades", "ada_1/missing", "discovery:homeassistant"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing topic %q in %v", want, sink.topics)
		}
	}
	if err := pub.FetchAndPublish(context.Background(), settings, mockFetcher{}, sink); err != nil {
		t.Fatal(err)
	}
	discoveryCount := 0
	for _, topic := range sink.topics {
		if strings.HasPrefix(topic, "discovery:") {
			discoveryCount++
		}
	}
	if discoveryCount != 1 {
		t.Fatalf("expected discovery once, got %d", discoveryCount)
	}
}
