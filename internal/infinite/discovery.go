package infinite

import (
	"fmt"
	"strings"

	"github.com/resnostyle/mqttkit/hadisc"
	"github.com/resnostyle/mqttkit/mqttpub"
)

const (
	deviceManufacturer = "infinite-mqtt"
	deviceUID          = "infinite_mqtt"
)

func deviceBlock() map[string]any {
	return hadisc.Device([]string{deviceUID}, "Infinite Campus MQTT", deviceManufacturer, "Parent Portal")
}

// BuildDiscoveryConfigs builds HA MQTT discovery configs for status + students.
func BuildDiscoveryConfigs(topicPrefix string, snapshots []StudentSnapshot) []mqttpub.Config {
	device := deviceBlock()
	statusTopic := mqttpub.Join(topicPrefix, "status")

	configs := []mqttpub.Config{
		{
			ObjectID:  deviceUID + "_connected",
			Component: "binary_sensor",
			Payload: map[string]any{
				"name":                  "Infinite Campus Connected",
				"unique_id":             deviceUID + "_connected",
				"state_topic":           statusTopic,
				"value_template":        "{{ value_json.connected }}",
				"payload_on":            "true",
				"payload_off":           "false",
				"device":                device,
				"object_id":             deviceUID + "_connected",
				"icon":                  "mdi:school",
				"json_attributes_topic": statusTopic,
			},
		},
		{
			ObjectID:  deviceUID + "_last_updated",
			Component: "sensor",
			Payload: map[string]any{
				"name":                  "Infinite Campus Last Updated",
				"unique_id":             deviceUID + "_last_updated",
				"state_topic":           statusTopic,
				"value_template":        "{{ value_json.last_updated }}",
				"device":                device,
				"object_id":             deviceUID + "_last_updated",
				"device_class":          "timestamp",
				"icon":                  "mdi:clock-check-outline",
				"json_attributes_topic": statusTopic,
			},
		},
	}

	for _, snap := range snapshots {
		configs = append(configs, studentDiscovery(topicPrefix, device, snap)...)
	}
	return configs
}

func studentDiscovery(topicPrefix string, device map[string]any, snap StudentSnapshot) []mqttpub.Config {
	prefix := "infinite_campus_" + snap.Slug
	missingTopic := mqttpub.Join(topicPrefix, snap.Slug+"/missing")
	upcomingTopic := mqttpub.Join(topicPrefix, snap.Slug+"/upcoming")
	summaryTopic := mqttpub.Join(topicPrefix, snap.Slug+"/summary")
	gradesTopic := mqttpub.Join(topicPrefix, snap.Slug+"/grades")
	display := strings.TrimSpace(snap.FirstName)
	if display == "" {
		display = snap.Slug
	}

	configs := []mqttpub.Config{
		{
			ObjectID:  prefix + "_missing_assignments",
			Component: "sensor",
			Payload: map[string]any{
				"name":                  fmt.Sprintf("%s Missing Assignments", display),
				"unique_id":             prefix + "_missing_assignments",
				"state_topic":           missingTopic,
				"value_template":        "{{ value_json.count }}",
				"device":                device,
				"object_id":             prefix + "_missing_assignments",
				"icon":                  "mdi:clipboard-alert",
				"json_attributes_topic": missingTopic,
				"unit_of_measurement":   "assignments",
				"state_class":           "measurement",
			},
		},
		{
			ObjectID:  prefix + "_upcoming_assignments",
			Component: "sensor",
			Payload: map[string]any{
				"name":                  fmt.Sprintf("%s Upcoming Assignments", display),
				"unique_id":             prefix + "_upcoming_assignments",
				"state_topic":           upcomingTopic,
				"value_template":        "{{ value_json.count }}",
				"device":                device,
				"object_id":             prefix + "_upcoming_assignments",
				"icon":                  "mdi:calendar-clock",
				"json_attributes_topic": upcomingTopic,
				"unit_of_measurement":   "assignments",
				"state_class":           "measurement",
			},
		},
		{
			ObjectID:  prefix + "_total_assignments",
			Component: "sensor",
			Payload: map[string]any{
				"name":                  fmt.Sprintf("%s Total Assignments", display),
				"unique_id":             prefix + "_total_assignments",
				"state_topic":           summaryTopic,
				"value_template":        "{{ value_json.assignment_count }}",
				"device":                device,
				"object_id":             prefix + "_total_assignments",
				"icon":                  "mdi:clipboard-text",
				"json_attributes_topic": summaryTopic,
				"unit_of_measurement":   "assignments",
				"state_class":           "measurement",
			},
		},
	}

	for i, g := range snap.Grades {
		courseSlug := CourseSlug(g.CourseName, g.SectionID)
		objectID := prefix + "_" + courseSlug
		// Jinja index into grades array published as { "courses": [...] }
		valueTemplate := fmt.Sprintf("{{ value_json.courses[%d].letter }}", i)
		configs = append(configs, mqttpub.Config{
			ObjectID:  objectID,
			Component: "sensor",
			Payload: map[string]any{
				"name":                  fmt.Sprintf("%s %s", display, g.CourseName),
				"unique_id":             objectID,
				"state_topic":           gradesTopic,
				"value_template":        valueTemplate,
				"device":                device,
				"object_id":             objectID,
				"icon":                  "mdi:school-outline",
				"json_attributes_topic": gradesTopic,
				"json_attributes_template": fmt.Sprintf(
					"{{ {'percent': value_json.courses[%d].percent, 'score': value_json.courses[%d].score, 'teacher': value_json.courses[%d].teacher, 'term_name': value_json.courses[%d].term_name, 'course_name': value_json.courses[%d].course_name} | tojson }}",
					i, i, i, i, i,
				),
			},
		})
	}
	return configs
}
