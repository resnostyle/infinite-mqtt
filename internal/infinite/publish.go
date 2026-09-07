package infinite

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/resnostyle/infinite-mqtt/internal/lib/campus"
	"github.com/resnostyle/mqttkit/mqttpub"
)

// Fetcher abstracts the campus portal for tests.
type Fetcher interface {
	Login(ctx context.Context) error
	Students(ctx context.Context) ([]campus.Student, error)
	Grades(ctx context.Context, student campus.Student) ([]campus.CourseGrade, error)
	Assignments(ctx context.Context, personID int) ([]campus.Assignment, error)
	Roster(ctx context.Context, personID int) ([]campus.RosterCourse, error)
}

// Publisher holds discovery fingerprint state across polls.
type Publisher struct {
	lastRosterKey string
}

func NewPublisher() *Publisher {
	return &Publisher{}
}

// FetchAndPublish logs in, pulls grades/assignments, and publishes retained topics.
func (p *Publisher) FetchAndPublish(ctx context.Context, settings Settings, fetcher Fetcher, mqtt mqttpub.Sink) error {
	if err := fetcher.Login(ctx); err != nil {
		_ = publishStatus(mqtt, StatusPayload{
			Connected:    false,
			LastUpdated:  time.Now().UTC().Format(time.RFC3339),
			StudentCount: 0,
			Error:        err.Error(),
		})
		return err
	}

	students, err := fetcher.Students(ctx)
	if err != nil {
		_ = publishStatus(mqtt, StatusPayload{
			Connected:   false,
			LastUpdated: time.Now().UTC().Format(time.RFC3339),
			Error:       err.Error(),
		})
		return err
	}

	now := time.Now()
	snapshots := make([]StudentSnapshot, 0, len(students))
	list := make([]StudentListItem, 0, len(students))

	for _, student := range students {
		grades, gerr := fetcher.Grades(ctx, student)
		if gerr != nil {
			slog.Warn("grades fetch failed", "student", student.PersonID, "err", gerr)
			grades = nil
		}
		roster, rerr := fetcher.Roster(ctx, student.PersonID)
		if rerr != nil {
			slog.Warn("roster fetch failed", "student", student.PersonID, "err", rerr)
		} else {
			grades = campus.EnrichGrades(grades, roster)
		}
		assignments, aerr := fetcher.Assignments(ctx, student.PersonID)
		if aerr != nil {
			slog.Warn("assignments fetch failed", "student", student.PersonID, "err", aerr)
			assignments = nil
		}

		snap := BuildSnapshot(student, grades, assignments, settings.UpcomingDays, now)
		snapshots = append(snapshots, snap)
		list = append(list, StudentListItem{
			Slug:      snap.Slug,
			PersonID:  snap.PersonID,
			FirstName: snap.FirstName,
			LastName:  snap.LastName,
			Grade:     snap.Grade,
			School:    snap.School,
		})

		if err := publishStudent(mqtt, snap); err != nil {
			return err
		}
	}

	if err := mqtt.Publish("students", map[string]any{"students": list}, true); err != nil {
		return fmt.Errorf("publish students: %w", err)
	}

	status := StatusPayload{
		Connected:    true,
		LastUpdated:  now.UTC().Format(time.RFC3339),
		StudentCount: len(snapshots),
	}
	if err := publishStatus(mqtt, status); err != nil {
		return err
	}

	if settings.MQTTDiscoveryEnabled {
		key := RosterKey(snapshots)
		if key != p.lastRosterKey {
			configs := BuildDiscoveryConfigs(settings.MQTTTopicPrefix, snapshots)
			if err := mqtt.PublishDiscovery(configs, settings.MQTTDiscoveryPrefix); err != nil {
				return fmt.Errorf("publish discovery: %w", err)
			}
			p.lastRosterKey = key
			slog.Info("published mqtt discovery", "entities", len(configs))
		}
	}

	slog.Info("published campus snapshot",
		"students", len(snapshots),
		"upcoming_days", settings.UpcomingDays,
	)
	return nil
}

func publishStatus(mqtt mqttpub.Sink, status StatusPayload) error {
	if err := mqtt.Publish("status", status, true); err != nil {
		return fmt.Errorf("publish status: %w", err)
	}
	return nil
}

func publishStudent(mqtt mqttpub.Sink, snap StudentSnapshot) error {
	base := snap.Slug
	if err := mqtt.Publish(base+"/grades", map[string]any{
		"person_id": snap.PersonID,
		"slug":      snap.Slug,
		"courses":   snap.Grades,
	}, true); err != nil {
		return fmt.Errorf("publish grades: %w", err)
	}
	if err := mqtt.Publish(base+"/assignments", map[string]any{
		"person_id":   snap.PersonID,
		"slug":        snap.Slug,
		"count":       len(snap.Assignments),
		"assignments": snap.Assignments,
	}, true); err != nil {
		return fmt.Errorf("publish assignments: %w", err)
	}
	if err := mqtt.Publish(base+"/missing", map[string]any{
		"person_id":   snap.PersonID,
		"slug":        snap.Slug,
		"count":       len(snap.Missing),
		"assignments": snap.Missing,
	}, true); err != nil {
		return fmt.Errorf("publish missing: %w", err)
	}
	if err := mqtt.Publish(base+"/upcoming", map[string]any{
		"person_id":   snap.PersonID,
		"slug":        snap.Slug,
		"count":       len(snap.Upcoming),
		"assignments": snap.Upcoming,
	}, true); err != nil {
		return fmt.Errorf("publish upcoming: %w", err)
	}
	if err := mqtt.Publish(base+"/summary", snap.Summary, true); err != nil {
		return fmt.Errorf("publish summary: %w", err)
	}
	return nil
}

// PublishBootstrapDiscovery publishes connection sensors before the first fetch.
func PublishBootstrapDiscovery(settings Settings, mqtt mqttpub.Sink) error {
	if !settings.MQTTDiscoveryEnabled {
		return nil
	}
	configs := BuildDiscoveryConfigs(settings.MQTTTopicPrefix, nil)
	return mqtt.PublishDiscovery(configs, settings.MQTTDiscoveryPrefix)
}
