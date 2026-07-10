package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"flowscale/internal/models"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) do(method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(res.Body)
		return fmt.Errorf("api error %d: %s", res.StatusCode, string(respBytes))
	}

	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

// Workflows
func (c *Client) ListWorkflows() ([]models.Workflow, error) {
	var out []models.Workflow
	err := c.do("GET", "/workflows", nil, &out)
	return out, err
}

func (c *Client) StartWorkflow(id string) (*models.WorkflowExecution, error) {
	var out models.WorkflowExecution
	req := map[string]string{"workflow_id": id}
	err := c.do("POST", "/workflows/start", req, &out)
	return &out, err
}

// Executions
func (c *Client) ListExecutions() ([]models.WorkflowExecution, error) {
	var out []models.WorkflowExecution
	err := c.do("GET", "/executions", nil, &out)
	return out, err
}

func (c *Client) GetExecution(id string) (*models.WorkflowExecution, error) {
	var out models.WorkflowExecution
	err := c.do("GET", "/executions/"+id, nil, &out)
	return &out, err
}

func (c *Client) GetExecutionEvents(id string) ([]models.WorkflowEvent, error) {
	var out []models.WorkflowEvent
	err := c.do("GET", "/executions/"+id+"/events", nil, &out)
	return out, err
}

// DLQ
func (c *Client) ListDLQ() ([]models.ActivityExecution, error) {
	var out []models.ActivityExecution
	err := c.do("GET", "/activities/dlq", nil, &out)
	return out, err
}

func (c *Client) RetryDLQ(id string) error {
	return c.do("POST", "/activities/dlq/"+id+"/retry", nil, nil)
}

// Schedules
func (c *Client) ListSchedules() ([]models.Schedule, error) {
	var out []models.Schedule
	err := c.do("GET", "/schedules", nil, &out)
	return out, err
}
