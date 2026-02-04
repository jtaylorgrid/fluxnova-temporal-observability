package fluxnova

import (
	"context"
	"log"
	"time"
)

// ProcessEvent represents a CDC event for a process instance
type ProcessEvent struct {
	EventType         string                   `json:"event_type"`
	ProcessInstanceID string                   `json:"process_instance_id"`
	ProcessDefinition string                   `json:"process_definition_key"`
	BusinessKey       *string                  `json:"business_key,omitempty"`
	State             string                   `json:"state"`
	StartTime         string                   `json:"start_time"`
	EndTime           *string                  `json:"end_time,omitempty"`
	DurationMillis    *int64                   `json:"duration_millis,omitempty"`
	Activities        []HistoricActivityInstance `json:"activities,omitempty"`
	Variables         map[string]any           `json:"variables,omitempty"`
	Timestamp         time.Time                `json:"timestamp"`
}

// Poller polls Fluxnova for process history
type Poller struct {
	client       *Client
	batchSize    int
	lastPollTime *time.Time
}

// NewPoller creates a new Fluxnova poller
func NewPoller(client *Client, batchSize int) *Poller {
	return &Poller{
		client:    client,
		batchSize: batchSize,
	}
}

// Poll fetches new process instances and their history
func (p *Poller) Poll(ctx context.Context) ([]ProcessEvent, error) {
	processes, err := p.client.GetHistoricProcessInstances(p.lastPollTime, p.batchSize)
	if err != nil {
		return nil, err
	}

	if len(processes) == 0 {
		return nil, nil
	}

	var events []ProcessEvent
	for _, proc := range processes {
		event := ProcessEvent{
			EventType:         "ProcessInstance",
			ProcessInstanceID: proc.ID,
			ProcessDefinition: proc.ProcessDefinitionKey,
			BusinessKey:       proc.BusinessKey,
			State:             proc.State,
			StartTime:         proc.StartTime,
			EndTime:           proc.EndTime,
			DurationMillis:    proc.DurationInMillis,
			Timestamp:         time.Now(),
		}

		// Fetch activities for this process
		activities, err := p.client.GetHistoricActivityInstances(proc.ID)
		if err != nil {
			log.Printf("Warning: failed to fetch activities for %s: %v", proc.ID, err)
		} else {
			event.Activities = activities
		}

		// Fetch variables for this process
		variables, err := p.client.GetHistoricVariableInstances(proc.ID)
		if err != nil {
			log.Printf("Warning: failed to fetch variables for %s: %v", proc.ID, err)
		} else {
			event.Variables = make(map[string]any)
			for _, v := range variables {
				event.Variables[v.Name] = v.Value
			}
		}

		events = append(events, event)

		// Update last poll time
		startTime, err := time.Parse("2006-01-02T15:04:05.000-0700", proc.StartTime)
		if err == nil {
			if p.lastPollTime == nil || startTime.After(*p.lastPollTime) {
				p.lastPollTime = &startTime
			}
		}
	}

	return events, nil
}

// SetCheckpoint sets the polling checkpoint
func (p *Poller) SetCheckpoint(t time.Time) {
	p.lastPollTime = &t
}

// GetCheckpoint returns the current polling checkpoint
func (p *Poller) GetCheckpoint() *time.Time {
	return p.lastPollTime
}
