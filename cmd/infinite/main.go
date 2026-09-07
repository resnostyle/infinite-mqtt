package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/resnostyle/infinite-mqtt/internal/infinite"
	"github.com/resnostyle/infinite-mqtt/internal/lib/campus"
	"github.com/resnostyle/mqttkit/logx"
	"github.com/resnostyle/mqttkit/mqttpub"
	"github.com/resnostyle/mqttkit/poll"
)

func main() {
	settings, err := infinite.FromEnv()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	logx.Configure(settings.LogLevel, false)

	slog.Info("starting infinite-mqtt",
		"base_url", settings.BaseURL,
		"district", settings.District,
		"poll_interval", settings.PollIntervalSeconds,
		"upcoming_days", settings.UpcomingDays,
		"mqtt", settings.MQTTHost,
		"port", settings.MQTTPort,
		"discovery", settings.MQTTDiscoveryEnabled,
	)

	ctx, cancel := poll.NotifyContext()
	defer cancel()

	mqtt, err := mqttpub.New(
		settings.MQTTHost,
		settings.MQTTPort,
		settings.MQTTClientID,
		settings.MQTTUsername,
		settings.MQTTPassword,
		settings.MQTTTopicPrefix,
	)
	if err != nil {
		slog.Error("mqtt connect failed", "err", err)
		os.Exit(1)
	}
	defer mqtt.Close()

	if err := infinite.PublishBootstrapDiscovery(settings, mqtt); err != nil {
		slog.Error("mqtt discovery publish failed", "err", err)
	}

	client, err := campus.New(settings.BaseURL, settings.Username, settings.Password, settings.District)
	if err != nil {
		slog.Error("campus client", "err", err)
		os.Exit(1)
	}

	pub := infinite.NewPublisher()
	interval := time.Duration(settings.PollIntervalSeconds) * time.Second

	for ctx.Err() == nil {
		if err := pub.FetchAndPublish(ctx, settings, client, mqtt); err != nil {
			slog.Error("fetch/publish failed", "err", err)
		}
		poll.Wait(ctx, interval)
	}
}
