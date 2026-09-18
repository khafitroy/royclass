package pocketbase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Client struct {
	BaseURL           string
	SuperUserEmail    string
	SuperUserPassword string
	token             string
	HTTP              *http.Client
}

func NewClient() *Client {
	return &Client{
		BaseURL:           getEnv("PB_URL", "http://127.0.0.1:8090"),
		SuperUserEmail:    getEnv("PB_SUPERUSER_EMAIL", "admin@gezyclass.web.id"),
		SuperUserPassword: getEnv("PB_SUPERUSER_PASSWORD", ""),
		HTTP:              &http.Client{},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (c *Client) auth() error {
	body := map[string]string{
		"identity": c.SuperUserEmail,
		"password": c.SuperUserPassword,
	}
	b, _ := json.Marshal(body)
	resp, err := c.HTTP.Post(
		c.BaseURL+"/api/collections/_superusers/auth-with-password",
		"application/json",
		bytes.NewReader(b),
	)
	if err != nil {
		return fmt.Errorf("auth failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("auth response decode failed: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("auth failed with HTTP %d: %v", resp.StatusCode, result["message"])
	}
	t, ok := result["token"].(string)
	if !ok {
		return fmt.Errorf("no token in response")
	}
	c.token = t
	return nil
}

func (c *Client) do(method, path string, body interface{}) (map[string]interface{}, error) {
	if c.token == "" {
		if err := c.auth(); err != nil {
			return nil, err
		}
	}

	url := c.BaseURL + path
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return map[string]interface{}{}, nil
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("PocketBase response decode failed: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := result["message"]
		return nil, fmt.Errorf("PocketBase request failed with HTTP %d: %v", resp.StatusCode, message)
	}
	return result, nil
}

func (c *Client) Create(collection string, record map[string]interface{}) (map[string]interface{}, error) {
	return c.do("POST", "/api/collections/"+collection+"/records", record)
}

func (c *Client) Update(collection string, id string, record map[string]interface{}) (map[string]interface{}, error) {
	return c.do("PATCH", "/api/collections/"+collection+"/records/"+id, record)
}

func (c *Client) List(collection string, filter string) ([]map[string]interface{}, error) {
	path := "/api/collections/" + collection + "/records?perPage=500"
	if filter != "" {
		path += "&filter=" + url.QueryEscape(filter)
	}
	result, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	return recordsFromResult(result), nil
}

// ListAll returns every record, paging through PocketBase's 500-record page
// limit. Existing callers that intentionally use List remain unchanged.
func (c *Client) ListAll(collection string, filter string) ([]map[string]interface{}, error) {
	var records []map[string]interface{}
	for page := 1; ; page++ {
		path := "/api/collections/" + collection + "/records?page=" + strconv.Itoa(page) + "&perPage=500"
		if filter != "" {
			path += "&filter=" + url.QueryEscape(filter)
		}
		result, err := c.do("GET", path, nil)
		if err != nil {
			return nil, err
		}
		pageRecords := recordsFromResult(result)
		records = append(records, pageRecords...)

		totalPages := 0
		if value, ok := result["totalPages"].(float64); ok {
			totalPages = int(value)
		}
		if len(pageRecords) == 0 || (totalPages > 0 && page >= totalPages) || (totalPages == 0 && len(pageRecords) < 500) {
			return records, nil
		}
	}
}

func recordsFromResult(result map[string]interface{}) []map[string]interface{} {
	items, _ := result["items"].([]interface{})
	var records []map[string]interface{}
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			records = append(records, m)
		}
	}
	return records
}

func (c *Client) GetOrCreate(collection string, record map[string]interface{}, filter string) (map[string]interface{}, error) {
	records, err := c.List(collection, filter)
	if err != nil {
		return nil, err
	}
	if len(records) > 0 {
		return records[0], nil
	}
	return c.Create(collection, record)
}

func Slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
