# infinite-mqtt

Go service that polls the Infinite Campus **Parent Portal** for grades and assignments, then publishes retained JSON to Mosquitto with optional Home Assistant MQTT discovery.

No Home Assistant credentials or custom integrations required — HA only needs MQTT configured to subscribe and run automations.

Inspired by [ha-infinite-campus](https://github.com/macguy81/ha-infinite-campus) sensor/API patterns and [ic_parent_api](https://github.com/schwartzpub/ic_parent_api) portal auth.

## Setup

1. Copy `.env.example` to `.env`.
2. Set MQTT broker settings.
3. Set Infinite Campus credentials:
   - **IC_BASE_URL** — your district login host (e.g. `https://downingtownpa.infinitecampus.org`), no path.
   - **IC_DISTRICT** — the `appName` value from the login page (View Source / Inspect → search for `appName`).
   - **IC_USERNAME** / **IC_PASSWORD** — parent portal login.

```bash
mise run infinite   # go run ./cmd/infinite with .env
# or
docker compose up -d
```

Image: `ghcr.io/resnostyle/infinite-mqtt:latest`

## Topics

Default prefix: `home/infinite`

| Topic | Contents |
|-------|----------|
| `home/infinite/status` | `connected`, `last_updated`, `student_count` |
| `home/infinite/students` | Linked students (id, name, grade, school) |
| `home/infinite/{student}/grades` | Course grades (`courses[]` with letter, percent, teacher, term) |
| `home/infinite/{student}/assignments` | Full assignment list |
| `home/infinite/{student}/missing` | `count` + missing assignments |
| `home/infinite/{student}/upcoming` | Assignments due within `IC_UPCOMING_DAYS` (default 7) |
| `home/infinite/{student}/summary` | Aggregates: missing/upcoming/graded counts |

`{student}` is a stable slug: lowercase first name + person ID (e.g. `ada_12345`).

## Home Assistant discovery

When `MQTT_DISCOVERY_ENABLED=true`, entities appear under device **Infinite Campus MQTT**:

- `binary_sensor.infinite_mqtt_connected`
- `sensor.infinite_mqtt_last_updated`
- Per student: `sensor.infinite_campus_{student}_missing_assignments`
- Per student: `sensor.infinite_campus_{student}_upcoming_assignments`
- Per student: `sensor.infinite_campus_{student}_total_assignments`
- Per course: `sensor.infinite_campus_{student}_{course}` — letter grade as state; percent/score/teacher as attributes

Discovery is republished when the student/course roster changes.

## Example automations

```yaml
# Notify when a student has a new missing assignment
trigger:
  - platform: mqtt
    topic: home/infinite/ada_12345/missing
condition:
  - condition: template
    value_template: "{{ trigger.payload_json.count | int > 0 }}"
action:
  - service: notify.mobile_app
    data:
      title: "Missing assignment"
      message: >
        {{ trigger.payload_json.assignments[0].course_name }}:
        {{ trigger.payload_json.assignments[0].name }}
```

```yaml
# Alert when a course grade drops below 70%
trigger:
  - platform: mqtt
    topic: home/infinite/ada_12345/grades
condition:
  - condition: template
    value_template: >
      {{ trigger.payload_json.courses
         | selectattr('percent', 'defined')
         | selectattr('percent', 'lt', 70)
         | list | count > 0 }}
action:
  - service: notify.mobile_app
    data:
      title: "Grade alert"
      message: "A course is below 70%"
```

## Development

```bash
mise run test
mise run build
```

## Scope

v1 covers **grades** and **assignments / due dates** only. Attendance, calendar, and WhatsApp alerts are out of scope.
