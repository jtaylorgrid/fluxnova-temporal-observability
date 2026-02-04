package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/refset/fluxnova-decision-observability/internal/config"
	"github.com/refset/fluxnova-decision-observability/internal/fluxnova"
	"github.com/refset/fluxnova-decision-observability/internal/kafka"
)

// Pipeline orchestrates the CDC flow from Fluxnova to Kafka
type Pipeline struct {
	cfg      *config.Config
	client   *fluxnova.Client
	poller   *fluxnova.Poller
	producer *kafka.Producer
}

// New creates a new pipeline
func New(cfg *config.Config) (*Pipeline, error) {
	client := fluxnova.NewClient(
		cfg.Fluxnova.BaseURL,
		cfg.Fluxnova.Username,
		cfg.Fluxnova.Password,
	)

	producer := kafka.NewProducer(
		cfg.Kafka.Brokers,
		cfg.Kafka.EventsTopic,
		cfg.Kafka.ProcessesTopic,
	)

	poller := fluxnova.NewPoller(client, cfg.Pipeline.BatchSize)

	return &Pipeline{
		cfg:      cfg,
		client:   client,
		poller:   poller,
		producer: producer,
	}, nil
}

// Run starts the CDC pipeline
func (p *Pipeline) Run(ctx context.Context) error {
	log.Printf("Starting Fluxnova CDC pipeline")
	log.Printf("  Fluxnova: %s", p.cfg.Fluxnova.BaseURL)
	log.Printf("  Kafka: %v", p.cfg.Kafka.Brokers)
	log.Printf("  Poll interval: %s", p.cfg.Pipeline.PollInterval)

	// Check Fluxnova connectivity
	if err := p.client.Ping(); err != nil {
		return fmt.Errorf("failed to connect to Fluxnova: %w", err)
	}
	log.Printf("Connected to Fluxnova")

	ticker := time.NewTicker(p.cfg.Pipeline.PollInterval)
	defer ticker.Stop()

	// Initial poll
	if err := p.poll(ctx); err != nil {
		log.Printf("Initial poll error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("Shutting down pipeline")
			return p.producer.Close()
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				log.Printf("Poll error: %v", err)
			}
		}
	}
}

func (p *Pipeline) poll(ctx context.Context) error {
	events, err := p.poller.Poll(ctx)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		return nil
	}

	log.Printf("Polled %d process events from Fluxnova", len(events))

	for _, event := range events {
		// Send process instance to processes topic
		processRecord := map[string]any{
			"_id":                    event.ProcessInstanceID,
			"process_instance_id":   event.ProcessInstanceID,
			"process_definition_key": event.ProcessDefinition,
			"business_key":          event.BusinessKey,
			"state":                 event.State,
			"start_time":            event.StartTime,
			"end_time":              event.EndTime,
			"duration_millis":       event.DurationMillis,
			"variables":             event.Variables,
			"_valid_from":           event.StartTime,
		}

		if err := p.producer.SendProcess(ctx, event.ProcessInstanceID, processRecord); err != nil {
			log.Printf("Failed to send process %s: %v", event.ProcessInstanceID, err)
			continue
		}

		// Send each activity as an event
		for _, activity := range event.Activities {
			activityRecord := map[string]any{
				"_id":                  activity.ID,
				"process_instance_id":  activity.ProcessInstanceID,
				"activity_id":          activity.ActivityID,
				"activity_name":        activity.ActivityName,
				"activity_type":        activity.ActivityType,
				"execution_id":         activity.ExecutionID,
				"task_id":              activity.TaskID,
				"assignee":             activity.Assignee,
				"start_time":           activity.StartTime,
				"end_time":             activity.EndTime,
				"duration_millis":      activity.DurationInMillis,
				"canceled":             activity.Canceled,
				"_valid_from":          activity.StartTime,
			}

			// Include process variables in the activity event for decision context
			if len(event.Variables) > 0 {
				varsJSON, _ := json.Marshal(event.Variables)
				activityRecord["process_variables"] = string(varsJSON)
			}

			if err := p.producer.SendEvent(ctx, activity.ID, activityRecord); err != nil {
				log.Printf("Failed to send activity %s: %v", activity.ID, err)
			}
		}
	}

	return nil
}
