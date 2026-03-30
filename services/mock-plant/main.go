package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/solarops/mock-plant/logger"
	"github.com/solarops/mock-plant/plant"
	"github.com/solarops/shared/models"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	plantName := envOrDefault("PLANT_NAME", "Unnamed Plant")
	panelCount, _ := strconv.Atoi(envOrDefault("PLANT_PANELS", "10"))
	wattPerSec, _ := strconv.ParseFloat(envOrDefault("WATT_PER_SEC", "300"), 64)
	natsURL := envOrDefault("NATS_URL", nats.DefaultURL)
	logPath := envOrDefault("LOG_PATH", "/var/log/plant/data.log")

	p := plant.NewPlant(plantName, panelCount, wattPerSec)
	log.Printf("Plant started: %s (id=%s, panels=%d, watt=%g)", p.Name, p.ID, panelCount, wattPerSec)

	// Connect to NATS
	nc, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("NATS connect: %v", err)
	}
	defer nc.Close()
	log.Printf("Connected to NATS: %s", natsURL)

	// Publish plant status: online
	statusMsg, _ := json.Marshal(models.PlantInfo{
		PlantID:    p.ID,
		PlantName:  p.Name,
		Panels:     panelCount,
		WattPerSec: wattPerSec,
	})
	nc.Publish(fmt.Sprintf("plant.%s.status", p.ID), statusMsg)

	// Subscribe to commands
	cmdSubject := fmt.Sprintf("plant.%s.command", p.ID)
	nc.Subscribe(cmdSubject, func(msg *nats.Msg) {
		var cmd models.Command
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			log.Printf("Invalid command: %v", err)
			return
		}
		log.Printf("Command received: %s for panel %s", cmd.Command, cmd.PanelID)
		p.HandleCommand(cmd)
	})
	log.Printf("Subscribed to: %s", cmdSubject)

	// Setup file logger
	fileLog, err := logger.NewFileLogger(logPath)
	if err != nil {
		log.Printf("Warning: cannot open log file %s: %v (continuing without file logging)", logPath, err)
		fileLog = nil
	} else {
		defer fileLog.Close()
	}

	// Ticker: publish data every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Heartbeat: re-publish status every 30s so late-joining subscribers discover us
	statusTicker := time.NewTicker(30 * time.Second)
	defer statusTicker.Stop()
	statusSubject := fmt.Sprintf("plant.%s.status", p.ID)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-statusTicker.C:
			nc.Publish(statusSubject, statusMsg)
		case <-ticker.C:
			readings := p.GeneratePanelReadings()
			summary := p.GenerateSummary()

			// Publish each panel reading individually
			panelSubject := fmt.Sprintf("plant.%s.panel.data", p.ID)
			for _, reading := range readings {
				bytes, _ := json.Marshal(reading)
				nc.Publish(panelSubject, bytes)
				// Write each panel as separate log line
				if fileLog != nil {
					fileLog.Write(reading)
				}
			}

			// Publish plant summary
			summarySubject := fmt.Sprintf("plant.%s.summary", p.ID)
			summaryBytes, _ := json.Marshal(summary)
			nc.Publish(summarySubject, summaryBytes)

		case <-sigCh:
			log.Println("Shutting down...")
			// Publish offline status
			nc.Publish(fmt.Sprintf("plant.%s.status", p.ID), []byte(`{"status":"offline"}`))
			nc.Flush()
			return
		}
	}
}
