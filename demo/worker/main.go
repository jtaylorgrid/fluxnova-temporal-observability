package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/refset/fluxnova-decision-observability/demo/workflow"
	"github.com/refset/fluxnova-decision-observability/demo/xtdb"
)

const (
	WorkerID    = "customer-service-worker"
	LockTimeout = 30000 // 30 seconds in milliseconds
)

type ExternalTask struct {
	ID                  string         `json:"id"`
	WorkerID            string         `json:"workerId"`
	TopicName           string         `json:"topicName"`
	ProcessInstanceID   string         `json:"processInstanceId"`
	ProcessDefinitionID string         `json:"processDefinitionId"`
	ActivityID          string         `json:"activityId"`
	ActivityInstanceID  string         `json:"activityInstanceId"`
	ExecutionID         string         `json:"executionId"`
	Variables           map[string]any `json:"variables"`
}

type FetchRequest struct {
	WorkerID     string             `json:"workerId"`
	MaxTasks     int                `json:"maxTasks"`
	UsePriority  bool               `json:"usePriority"`
	Topics       []TopicSubscription `json:"topics"`
}

type TopicSubscription struct {
	TopicName    string `json:"topicName"`
	LockDuration int    `json:"lockDuration"`
}

type CompleteRequest struct {
	WorkerID  string         `json:"workerId"`
	Variables map[string]any `json:"variables,omitempty"`
}

type FailureRequest struct {
	WorkerID     string `json:"workerId"`
	ErrorMessage string `json:"errorMessage"`
	ErrorDetails string `json:"errorDetails"`
	Retries      int    `json:"retries"`
	RetryTimeout int    `json:"retryTimeout"`
}

func main() {
	baseURL := os.Getenv("FLUXNOVA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080/engine-rest"
	}

	var activities *workflow.Activities

	xtdbConnString := os.Getenv("XTDB_CONN_STRING")
	if xtdbConnString != "" {
		xtdbClient, err := xtdb.NewClientFromConnString(context.Background(), xtdbConnString)
		if err != nil {
			log.Println("Warning: Could not connect to XTDB, activities will not persist decision context:", err)
			activities = workflow.NewActivities()
		} else {
			defer xtdbClient.Close()
			activities = workflow.NewActivitiesWithXTDB(xtdbClient)
			log.Println("XTDB integration enabled - activities will persist decision context")
		}
	} else {
		activities = workflow.NewActivities()
		log.Println("XTDB_CONN_STRING not set - activities will not persist decision context")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received shutdown signal")
		cancel()
	}()

	log.Printf("Starting Fluxnova external task worker")
	log.Printf("  Fluxnova: %s", baseURL)
	log.Printf("  Worker ID: %s", WorkerID)

	topics := []TopicSubscription{
		{TopicName: "analyze-sentiment", LockDuration: LockTimeout},
		{TopicName: "lookup-customer-profile", LockDuration: LockTimeout},
		{TopicName: "check-churn-signals", LockDuration: LockTimeout},
		{TopicName: "decide-routing", LockDuration: LockTimeout},
		{TopicName: "generate-response", LockDuration: LockTimeout},
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Worker shutting down")
			return
		case <-ticker.C:
			tasks, err := fetchAndLock(baseURL, topics)
			if err != nil {
				log.Printf("Error fetching tasks: %v", err)
				continue
			}

			for _, task := range tasks {
				if err := handleTask(ctx, baseURL, activities, task); err != nil {
					log.Printf("Error handling task %s: %v", task.ID, err)
					failTask(baseURL, task.ID, err.Error())
				}
			}
		}
	}
}

func fetchAndLock(baseURL string, topics []TopicSubscription) ([]ExternalTask, error) {
	req := FetchRequest{
		WorkerID:    WorkerID,
		MaxTasks:    10,
		UsePriority: true,
		Topics:      topics,
	}

	body, _ := json.Marshal(req)
	resp, err := http.Post(baseURL+"/external-task/fetchAndLock", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var tasks []ExternalTask
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, err
	}

	return tasks, nil
}

func handleTask(ctx context.Context, baseURL string, activities *workflow.Activities, task ExternalTask) error {
	log.Printf("Handling task %s (topic: %s, process: %s)", task.ID, task.TopicName, task.ProcessInstanceID)

	var result map[string]any

	switch task.TopicName {
	case "analyze-sentiment":
		input := extractTicketInput(task.Variables)
		sentiment, err := activities.AnalyzeSentiment(ctx, task.ProcessInstanceID, input)
		if err != nil {
			return err
		}
		result = map[string]any{
			"sentiment": map[string]any{"value": toJSON(sentiment)},
		}

	case "lookup-customer-profile":
		customerID := getStringVar(task.Variables, "customerId")
		profile, err := activities.LookupCustomerProfile(ctx, task.ProcessInstanceID, customerID)
		if err != nil {
			return err
		}
		result = map[string]any{
			"customerProfile": map[string]any{"value": toJSON(profile)},
		}

	case "check-churn-signals":
		customerID := getStringVar(task.Variables, "customerId")
		churn, err := activities.CheckChurnSignals(ctx, task.ProcessInstanceID, customerID)
		if err != nil {
			return err
		}
		result = map[string]any{
			"churnSignals": map[string]any{"value": toJSON(churn)},
		}

	case "decide-routing":
		input := struct {
			Ticket    workflow.TicketInput
			Sentiment workflow.SentimentResult
			Profile   workflow.CustomerProfile
			Churn     workflow.ChurnSignals
		}{
			Ticket:    extractTicketInput(task.Variables),
			Sentiment: extractSentiment(task.Variables),
			Profile:   extractProfile(task.Variables),
			Churn:     extractChurn(task.Variables),
		}
		routing, err := activities.DecideRouting(ctx, task.ProcessInstanceID, input)
		if err != nil {
			return err
		}
		result = map[string]any{
			"routingDecision": map[string]any{"value": toJSON(routing)},
		}

	case "generate-response":
		input := struct {
			Ticket    workflow.TicketInput
			Sentiment workflow.SentimentResult
			Profile   workflow.CustomerProfile
			Routing   workflow.RoutingDecision
		}{
			Ticket:    extractTicketInput(task.Variables),
			Sentiment: extractSentiment(task.Variables),
			Profile:   extractProfile(task.Variables),
			Routing:   extractRouting(task.Variables),
		}
		response, err := activities.GenerateResponse(ctx, task.ProcessInstanceID, input)
		if err != nil {
			return err
		}
		result = map[string]any{
			"responseDraft": map[string]any{"value": toJSON(response)},
		}

	default:
		return fmt.Errorf("unknown topic: %s", task.TopicName)
	}

	return completeTask(baseURL, task.ID, result)
}

func completeTask(baseURL, taskID string, variables map[string]any) error {
	req := CompleteRequest{
		WorkerID:  WorkerID,
		Variables: variables,
	}

	body, _ := json.Marshal(req)
	resp, err := http.Post(baseURL+"/external-task/"+taskID+"/complete", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Completed task %s", taskID)
	return nil
}

func failTask(baseURL, taskID, errorMessage string) {
	req := FailureRequest{
		WorkerID:     WorkerID,
		ErrorMessage: errorMessage,
		Retries:      0,
		RetryTimeout: 0,
	}

	body, _ := json.Marshal(req)
	resp, err := http.Post(baseURL+"/external-task/"+taskID+"/failure", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("Failed to report failure for task %s: %v", taskID, err)
		return
	}
	defer resp.Body.Close()

	log.Printf("Reported failure for task %s", taskID)
}

func getStringVar(vars map[string]any, name string) string {
	if v, ok := vars[name]; ok {
		if m, ok := v.(map[string]any); ok {
			if val, ok := m["value"].(string); ok {
				return val
			}
		}
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getJSONVar(vars map[string]any, name string, target any) {
	if v, ok := vars[name]; ok {
		if m, ok := v.(map[string]any); ok {
			if val, ok := m["value"].(string); ok {
				json.Unmarshal([]byte(val), target)
				return
			}
		}
		if s, ok := v.(string); ok {
			json.Unmarshal([]byte(s), target)
		}
	}
}

func extractTicketInput(vars map[string]any) workflow.TicketInput {
	var ticket workflow.TicketInput
	getJSONVar(vars, "ticket", &ticket)
	if ticket.TicketID == "" {
		ticket.TicketID = getStringVar(vars, "ticketId")
		ticket.CustomerID = getStringVar(vars, "customerId")
		ticket.Subject = getStringVar(vars, "subject")
		ticket.Body = getStringVar(vars, "body")
		ticket.Channel = getStringVar(vars, "channel")
	}
	return ticket
}

func extractSentiment(vars map[string]any) workflow.SentimentResult {
	var result workflow.SentimentResult
	getJSONVar(vars, "sentiment", &result)
	return result
}

func extractProfile(vars map[string]any) workflow.CustomerProfile {
	var result workflow.CustomerProfile
	getJSONVar(vars, "customerProfile", &result)
	return result
}

func extractChurn(vars map[string]any) workflow.ChurnSignals {
	var result workflow.ChurnSignals
	getJSONVar(vars, "churnSignals", &result)
	return result
}

func extractRouting(vars map[string]any) workflow.RoutingDecision {
	var result workflow.RoutingDecision
	getJSONVar(vars, "routingDecision", &result)
	return result
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
