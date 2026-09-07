# infinite-mqtt

Go service that polls the Infinite Campus **Parent Portal** for grades and assignments, then publishes retained JSON to Mosquitto with optional Home Assistant MQTT discovery.

No Home Assistant credentials or custom integrations required — HA only needs MQTT configured to subscribe and run automations.

Uses shared infrastructure from [`mqttkit`](https://github.com/resnostyle/mqttkit). Inspired by [ha-infinite-campus](https://github.com/macguy81/ha-infinite-campus) sensor/API patterns and [ic_parent_api](https://github.com/schwartzpub/ic_parent_api) portal auth.

## Topics (default prefix `home/infinite`)

| Topic | Contents |
|-------|----------|
| `home/infinite/status` | `connected`, `last_updated`, `student_count` (and `error` on failure) |
| `home/infinite/students` | Linked students (slug, id, name, grade, school) |
| `home/infinite/{student}/grades` | Course grades (`courses[]` with letter, percent, teacher, term) |
| `home/infinite/{student}/assignments` | Full assignment list |
| `home/infinite/{student}/missing` | `count` + missing assignments |
| `home/infinite/{student}/upcoming` | Assignments due within `IC_UPCOMING_DAYS` (default 7) |
| `home/infinite/{student}/summary` | Aggregates: course/assignment/missing/upcoming/graded counts |

`{student}` is a stable slug: lowercase first name + person ID (e.g. `ada_12345`).

## Home Assistant

When `MQTT_DISCOVERY_ENABLED=true`, entities appear under device **Infinite Campus MQTT**:

- `binary_sensor.infinite_mqtt_connected`
- `sensor.infinite_mqtt_last_updated`
- Per student: missing / upcoming / total assignment sensors
- Per course: letter grade as state; percent, score, teacher, and term as attributes

Discovery is republished when the student or course roster changes.

### Example automations

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

## Quick start

```bash
cp .env.example .env
# Set MQTT_* and IC_* (see Configuration)

mise run infinite   # go run ./cmd/infinite with .env
# or
docker compose up -d
```

Image: `ghcr.io/resnostyle/infinite-mqtt:latest`

## Configuration

See [`.env.example`](.env.example). Required Infinite Campus settings:

| Variable | Description |
|----------|-------------|
| `IC_BASE_URL` | District login host, no path (e.g. `https://downingtownpa.infinitecampus.org`) |
| `IC_DISTRICT` | `appName` from the login page (View Source / Inspect → search for `appName`) |
| `IC_USERNAME` / `IC_PASSWORD` | Parent portal login |
| `IC_POLL_INTERVAL_SECONDS` | Poll interval (default `900`, minimum `60`) |
| `IC_UPCOMING_DAYS` | Upcoming assignment horizon (default `7`) |

MQTT defaults: `MQTT_HOST=127.0.0.1`, `MQTT_PORT=1883`, `MQTT_TOPIC_PREFIX=home/infinite`, discovery on under `homeassistant`.

## Development

```bash
mise run test
mise run build
```

## Scope

v1 covers **grades** and **assignments / due dates** only. Attendance, calendar, and WhatsApp alerts are out of scope.
