package fluxnova

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a Fluxnova/Camunda 7 REST API client
type Client struct {
	baseURL    string
	httpClient *http.Client
	authHeader string
}

// NewClient creates a new Fluxnova client
func NewClient(baseURL, username, password string) *Client {
	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	if username != "" && password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		c.authHeader = "Basic " + auth
	}
	return c
}

// HistoricProcessInstance represents a completed or running process instance
type HistoricProcessInstance struct {
	ID                       string  `json:"id"`
	BusinessKey              *string `json:"businessKey"`
	ProcessDefinitionID      string  `json:"processDefinitionId"`
	ProcessDefinitionKey     string  `json:"processDefinitionKey"`
	ProcessDefinitionName    *string `json:"processDefinitionName"`
	ProcessDefinitionVersion int     `json:"processDefinitionVersion"`
	StartTime                string  `json:"startTime"`
	EndTime                  *string `json:"endTime"`
	DurationInMillis         *int64  `json:"durationInMillis"`
	StartUserID              *string `json:"startUserId"`
	StartActivityID          string  `json:"startActivityId"`
	DeleteReason             *string `json:"deleteReason"`
	SuperProcessInstanceID   *string `json:"superProcessInstanceId"`
	TenantID                 *string `json:"tenantId"`
	State                    string  `json:"state"`
}

// HistoricActivityInstance represents a completed activity
type HistoricActivityInstance struct {
	ID                       string  `json:"id"`
	ParentActivityInstanceID *string `json:"parentActivityInstanceId"`
	ActivityID               string  `json:"activityId"`
	ActivityName             *string `json:"activityName"`
	ActivityType             string  `json:"activityType"`
	ProcessDefinitionKey     string  `json:"processDefinitionKey"`
	ProcessDefinitionID      string  `json:"processDefinitionId"`
	ProcessInstanceID        string  `json:"processInstanceId"`
	ExecutionID              string  `json:"executionId"`
	TaskID                   *string `json:"taskId"`
	Assignee                 *string `json:"assignee"`
	CalledProcessInstanceID  *string `json:"calledProcessInstanceId"`
	CalledCaseInstanceID     *string `json:"calledCaseInstanceId"`
	StartTime                string  `json:"startTime"`
	EndTime                  *string `json:"endTime"`
	DurationInMillis         *int64  `json:"durationInMillis"`
	Canceled                 bool    `json:"canceled"`
	CompleteScope            bool    `json:"completeScope"`
	TenantID                 *string `json:"tenantId"`
}

// HistoricVariableInstance represents a process variable value
type HistoricVariableInstance struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Type                string  `json:"type"`
	Value               any     `json:"value"`
	ProcessDefinitionID string  `json:"processDefinitionId"`
	ProcessInstanceID   string  `json:"processInstanceId"`
	ExecutionID         *string `json:"executionId"`
	ActivityInstanceID  *string `json:"activityInstanceId"`
	TaskID              *string `json:"taskId"`
	CreateTime          string  `json:"createTime"`
	State               string  `json:"state"`
	TenantID            *string `json:"tenantId"`
}

// HistoricDetail represents a detailed audit log entry
type HistoricDetail struct {
	ID                  string  `json:"id"`
	Type                string  `json:"type"`
	ProcessDefinitionID string  `json:"processDefinitionId"`
	ProcessInstanceID   string  `json:"processInstanceId"`
	ExecutionID         *string `json:"executionId"`
	ActivityInstanceID  *string `json:"activityInstanceId"`
	TaskID              *string `json:"taskId"`
	Time                string  `json:"time"`
	TenantID            *string `json:"tenantId"`
	// For variable updates
	VariableName         *string `json:"variableName,omitempty"`
	VariableInstanceID   *string `json:"variableInstanceId,omitempty"`
	VariableType         *string `json:"variableType,omitempty"`
	Value                any     `json:"value,omitempty"`
	Revision             *int    `json:"revision,omitempty"`
	InitialValue         *bool   `json:"initial,omitempty"`
}

// GetHistoricProcessInstances queries historic process instances
func (c *Client) GetHistoricProcessInstances(startedAfter *time.Time, maxResults int) ([]HistoricProcessInstance, error) {
	url := fmt.Sprintf("%s/history/process-instance?sortBy=startTime&sortOrder=asc&maxResults=%d", c.baseURL, maxResults)
	if startedAfter != nil {
		url += "&startedAfter=" + startedAfter.Format("2006-01-02T15:04:05.000-0700")
	}

	var result []HistoricProcessInstance
	if err := c.get(url, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetHistoricActivityInstances queries historic activity instances for a process
func (c *Client) GetHistoricActivityInstances(processInstanceID string) ([]HistoricActivityInstance, error) {
	url := fmt.Sprintf("%s/history/activity-instance?processInstanceId=%s&sortBy=startTime&sortOrder=asc", c.baseURL, processInstanceID)

	var result []HistoricActivityInstance
	if err := c.get(url, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetHistoricVariableInstances queries historic variable instances for a process
func (c *Client) GetHistoricVariableInstances(processInstanceID string) ([]HistoricVariableInstance, error) {
	url := fmt.Sprintf("%s/history/variable-instance?processInstanceId=%s", c.baseURL, processInstanceID)

	var result []HistoricVariableInstance
	if err := c.get(url, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetHistoricDetails queries historic details (audit log) for a process
func (c *Client) GetHistoricDetails(processInstanceID string) ([]HistoricDetail, error) {
	url := fmt.Sprintf("%s/history/detail?processInstanceId=%s&sortBy=time&sortOrder=asc", c.baseURL, processInstanceID)

	var result []HistoricDetail
	if err := c.get(url, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) get(url string, result any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Fluxnova API error %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// Ping checks connectivity to Fluxnova
func (c *Client) Ping() error {
	url := fmt.Sprintf("%s/engine", c.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Fluxnova ping failed with status %d", resp.StatusCode)
	}
	return nil
}
