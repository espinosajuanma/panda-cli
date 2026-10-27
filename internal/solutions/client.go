package solutions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type listResponse struct {
	Total  float64           `json:"total"`
	Offset string            `json:"offset"`
	Items  []json.RawMessage `json:"items"`
}

const taskListFields = "id,label,version,project,number,title,type,package,labels,priority,complexity,workload,status,assignees,reviewers,reviewFeedbacks,github,createdBy,createdAt,release,releaseInformation,releaseNotes,description,files,notes,checklist,rank,reviewStatus,relatedFiles"

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *Client) Login(ctx context.Context, email, password string) (string, error) {
	var response struct {
		Token string `json:"token"`
	}
	err := c.request(ctx, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": password,
	}, &response)
	if err != nil {
		return "", err
	}
	if response.Token == "" {
		return "", fmt.Errorf("login response did not include a token")
	}
	return response.Token, nil
}

func (c *Client) FindProject(ctx context.Context, name string) (*Project, error) {
	query := url.Values{"name": []string{name}}
	items, err := c.list(ctx, "/data/projects", query)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		var project Project
		if err := json.Unmarshal(item, &project); err != nil {
			return nil, err
		}
		if strings.EqualFold(project.Name, name) {
			return &project, nil
		}
	}
	return nil, fmt.Errorf("project %q was not found", name)
}

func (c *Client) ListTasks(ctx context.Context, projectID string) ([]map[string]any, error) {
	query := url.Values{
		"project.id": []string{projectID},
		"_fields":    []string{taskListFields},
	}
	items, err := c.list(ctx, "/data/dev.tasks", query)
	if err != nil {
		return nil, err
	}
	tasks := make([]map[string]any, 0, len(items))
	for _, item := range items {
		var task map[string]any
		if err := json.Unmarshal(item, &task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (c *Client) ListLabels(ctx context.Context, projectID string) ([]map[string]any, error) {
	query := url.Values{"project.id": []string{projectID}}
	items, err := c.list(ctx, "/data/dev.labels", query)
	if err != nil {
		return nil, err
	}
	labels := make([]map[string]any, 0, len(items))
	for _, item := range items {
		var label map[string]any
		if err := json.Unmarshal(item, &label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, nil
}

func (c *Client) GetTask(ctx context.Context, id string) (map[string]any, error) {
	var task map[string]any
	err := c.request(ctx, http.MethodGet, "/data/dev.tasks/"+url.PathEscape(id), nil, &task)
	return task, err
}

func (c *Client) CreateTask(ctx context.Context, task map[string]any) (map[string]any, error) {
	var created map[string]any
	err := c.request(ctx, http.MethodPost, "/data/dev.tasks", task, &created)
	return created, err
}

func (c *Client) UpdateTask(ctx context.Context, id string, task map[string]any) (map[string]any, error) {
	var updated map[string]any
	err := c.request(ctx, http.MethodPut, "/data/dev.tasks/"+url.PathEscape(id), task, &updated)
	return updated, err
}

func (c *Client) AddNote(ctx context.Context, id, note string) error {
	return c.request(ctx, http.MethodPut, "/data/dev.tasks/"+url.PathEscape(id)+"/addNote", map[string]string{"note": note}, nil)
}

func (c *Client) list(ctx context.Context, endpoint string, query url.Values) ([]json.RawMessage, error) {
	items := make([]json.RawMessage, 0)
	seenOffsets := make(map[string]bool)
	offset := ""
	for {
		var response listResponse
		path := endpoint
		params := make(url.Values, len(query)+1)
		for key, values := range query {
			params[key] = append([]string(nil), values...)
		}
		if offset != "" {
			params.Set("_offset", offset)
		}
		if encoded := params.Encode(); encoded != "" {
			path += "?" + encoded
		}
		if err := c.request(ctx, http.MethodGet, path, nil, &response); err != nil {
			return nil, err
		}
		items = append(items, response.Items...)
		if response.Total > 0 && float64(len(items)) >= response.Total {
			break
		}
		if len(response.Items) == 0 {
			break
		}
		if response.Offset == "" {
			break
		}
		if seenOffsets[response.Offset] {
			return nil, fmt.Errorf("API returned a repeated pagination offset")
		}
		seenOffsets[response.Offset] = true
		offset = response.Offset
	}
	return items, nil
}

func (c *Client) request(ctx context.Context, method, path string, body, output any) error {
	var requestBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		requestBody = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, requestBody)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		request.Header.Set("token", c.Token)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("Solutions API %s %s: %s", method, path, message)
	}
	if output == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode Solutions API response: %w", err)
	}
	return nil
}
