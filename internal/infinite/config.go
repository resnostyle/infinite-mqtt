package infinite

import (
	"fmt"
	"strings"

	"github.com/resnostyle/mqttkit/env"
)

const (
	defaultTopicPrefix = "home/infinite"
	defaultClientID    = "infinite-mqtt"
)

// Settings is runtime configuration for infinite-mqtt.
type Settings struct {
	env.MQTT
	BaseURL             string
	District            string
	Username            string
	Password            string
	PollIntervalSeconds int
	UpcomingDays        int
}

// FromEnv loads settings from the environment.
func FromEnv() (Settings, error) {
	mqtt, err := env.LoadMQTT(defaultTopicPrefix, defaultClientID)
	if err != nil {
		return Settings{}, err
	}
	baseURL, err := env.Require("IC_BASE_URL")
	if err != nil {
		return Settings{}, err
	}
	district, err := env.Require("IC_DISTRICT")
	if err != nil {
		return Settings{}, err
	}
	username, err := env.Require("IC_USERNAME")
	if err != nil {
		return Settings{}, err
	}
	password, err := env.Require("IC_PASSWORD")
	if err != nil {
		return Settings{}, err
	}
	poll, err := env.Int("IC_POLL_INTERVAL_SECONDS", 900)
	if err != nil {
		return Settings{}, err
	}
	if poll < 60 {
		return Settings{}, fmt.Errorf("IC_POLL_INTERVAL_SECONDS must be >= 60")
	}
	upcoming, err := env.Int("IC_UPCOMING_DAYS", 7)
	if err != nil {
		return Settings{}, err
	}
	if upcoming < 1 {
		return Settings{}, fmt.Errorf("IC_UPCOMING_DAYS must be >= 1")
	}
	return Settings{
		MQTT:                mqtt,
		BaseURL:             strings.TrimRight(baseURL, "/"),
		District:            district,
		Username:            username,
		Password:            password,
		PollIntervalSeconds: poll,
		UpcomingDays:        upcoming,
	}, nil
}
