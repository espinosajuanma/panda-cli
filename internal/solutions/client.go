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
	Email   string
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
	return c.listProjectRecords(ctx, "dev.labels", projectID)
}

func (c *Client) ListPackages(ctx context.Context, projectID string) ([]map[string]any, error) {
	return c.listProjectRecords(ctx, "dev.packages", projectID)
}

func (c *Client) listProjectRecords(ctx context.Context, entity, projectID string) ([]map[string]any, error) {
	items, err := c.list(ctx, "/data/"+entity, url.Values{"project.id": []string{projectID}})
	if err != nil {
		return nil, err
	}
	records := make([]map[string]any, 0, len(items))
	for _, item := range items {
		var record map[string]any
		if err := json.Unmarshal(item, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (c *Client) CreateLabel(ctx context.Context, label map[string]any) (map[string]any, error) {
	return c.createProjectRecord(ctx, "dev.labels", label)
}

func (c *Client) UpdateLabel(ctx context.Context, id string, label map[string]any) (map[string]any, error) {
	return c.updateProjectRecord(ctx, "dev.labels", id, label)
}

func (c *Client) DeleteLabel(ctx context.Context, id string) error {
	return c.deleteProjectRecord(ctx, "dev.labels", id)
}

func (c *Client) CreatePackage(ctx context.Context, pkg map[string]any) (map[string]any, error) {
	return c.createProjectRecord(ctx, "dev.packages", pkg)
}

func (c *Client) UpdatePackage(ctx context.Context, id string, pkg map[string]any) (map[string]any, error) {
	return c.updateProjectRecord(ctx, "dev.packages", id, pkg)
}

func (c *Client) DeletePackage(ctx context.Context, id string) error {
	return c.deleteProjectRecord(ctx, "dev.packages", id)
}

func (c *Client) createProjectRecord(ctx context.Context, entity string, record map[string]any) (map[string]any, error) {
	var created map[string]any
	err := c.request(ctx, http.MethodPost, "/data/"+entity, record, &created)
	return created, err
}

func (c *Client) updateProjectRecord(ctx context.Context, entity, id string, record map[string]any) (map[string]any, error) {
	var updated map[string]any
	err := c.request(ctx, http.MethodPut, "/data/"+entity+"/"+url.PathEscape(id), record, &updated)
	return updated, err
}

func (c *Client) deleteProjectRecord(ctx context.Context, entity, id string) error {
	return c.request(ctx, http.MethodDelete, "/data/"+entity+"/"+url.PathEscape(id), nil, nil)
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

func (c *Client) FindUser(ctx context.Context, email string) (map[string]any, error) {
	items, err := c.list(ctx, "/data/sys.users", url.Values{
		"email":   []string{email},
		"_fields": []string{"id,email,label"},
	})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		var user map[string]any
		if err := json.Unmarshal(item, &user); err != nil {
			return nil, err
		}
		if strings.EqualFold(stringValue(user["email"]), email) {
			return user, nil
		}
	}
	return nil, fmt.Errorf("user %q was not found", email)
}

func (c *Client) EditMyNotes(ctx context.Context, id string, myNotes []map[string]any) (map[string]any, error) {
	var updated map[string]any
	err := c.request(ctx, http.MethodPut, "/data/dev.tasks/"+url.PathEscape(id)+"/editMyNotes", map[string]any{
		"myNotes": myNotes,
	}, &updated)
	return updated, err
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
		if c.Token != "" && path != "/auth/login" && isInvalidTokenResponse(data) {
			return fmt.Errorf("your Solutions login token is invalid or expired; run `panda login` to sign in again")
		}
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

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func isInvalidTokenResponse(data []byte) bool {
	var response struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(response.Code))
	message := strings.ToLower(strings.TrimSpace(response.Message))
	if strings.Contains(message, "invalid token") || strings.Contains(message, "expired token") || strings.Contains(message, "token expired") {
		return true
	}
	return (code == "unauthorize" || code == "unauthorized") && strings.Contains(message, "token") && strings.Contains(message, "login")
}
