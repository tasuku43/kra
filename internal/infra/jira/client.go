package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	envJiraBaseURL  = "GIONX_JIRA_BASE_URL"
	envJiraEmail    = "GIONX_JIRA_EMAIL"
	envJiraAPIToken = "GIONX_JIRA_API_TOKEN"
)

var issueKeyRegexp = regexp.MustCompile(`(?i)\b([a-z][a-z0-9]+-\d+)\b`)
var legacySprintKVPattern = regexp.MustCompile(`([a-zA-Z]+)=([^,\]]+)`)

type Client struct {
	httpClient *http.Client
}

type Issue struct {
	Key       string
	Summary   string
	TicketURL string
}

type Board struct {
	ID         int
	Name       string
	Type       string
	ProjectKey string
}

type Sprint struct {
	ID            int
	Name          string
	State         string
	OriginBoardID int
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) FetchIssueByTicketURL(ctx context.Context, ticketURL string) (key string, summary string, err error) {
	cfg, err := loadEnvConfig()
	if err != nil {
		return "", "", err
	}
	issueKey, err := parseTicketURL(ticketURL)
	if err != nil {
		return "", "", err
	}

	endpoint := strings.TrimRight(cfg.baseURL.String(), "/") + "/rest/api/3/issue/" + url.PathEscape(issueKey) + "?fields=summary"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", fmt.Errorf("build jira request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuth(cfg.email, cfg.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("jira request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", "", fmt.Errorf("jira authentication failed: status=%d", resp.StatusCode)
	case http.StatusNotFound:
		return "", "", fmt.Errorf("jira issue not found: %s", issueKey)
	default:
		return "", "", fmt.Errorf("jira request failed: status=%d", resp.StatusCode)
	}

	var payload struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", fmt.Errorf("decode jira response: %w", err)
	}
	resolvedKey := strings.TrimSpace(payload.Key)
	if resolvedKey == "" {
		resolvedKey = issueKey
	}
	return strings.ToUpper(resolvedKey), strings.TrimSpace(payload.Fields.Summary), nil
}

type envConfig struct {
	baseURL  *url.URL
	email    string
	apiToken string
}

func loadEnvConfig() (envConfig, error) {
	baseURLRaw := strings.TrimSpace(os.Getenv(envJiraBaseURL))
	email := strings.TrimSpace(os.Getenv(envJiraEmail))
	apiToken := strings.TrimSpace(os.Getenv(envJiraAPIToken))

	missing := make([]string, 0, 3)
	if baseURLRaw == "" {
		missing = append(missing, envJiraBaseURL)
	}
	if email == "" {
		missing = append(missing, envJiraEmail)
	}
	if apiToken == "" {
		missing = append(missing, envJiraAPIToken)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return envConfig{}, fmt.Errorf("missing jira env vars: %s", strings.Join(missing, ", "))
	}

	baseURL, err := url.Parse(baseURLRaw)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return envConfig{}, fmt.Errorf("invalid %s: %q", envJiraBaseURL, baseURLRaw)
	}

	return envConfig{baseURL: baseURL, email: email, apiToken: apiToken}, nil
}

func parseTicketURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid jira ticket URL: %q", raw)
	}
	m := issueKeyRegexp.FindStringSubmatch(strings.ToUpper(raw))
	if len(m) < 2 {
		return "", fmt.Errorf("invalid jira ticket URL: %q", raw)
	}
	return strings.ToUpper(m[1]), nil
}

func basicAuth(email string, token string) string {
	return base64.StdEncoding.EncodeToString([]byte(email + ":" + token))
}

func (c *Client) SearchIssuesByJQL(ctx context.Context, jql string, maxResults int) ([]Issue, error) {
	cfg, err := loadEnvConfig()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(jql) == "" {
		return nil, fmt.Errorf("jql is required")
	}
	if maxResults <= 0 {
		maxResults = 50
	}

	q := url.Values{}
	q.Set("jql", jql)
	q.Set("maxResults", fmt.Sprintf("%d", maxResults))
	q.Set("fields", "summary")
	endpoint := strings.TrimRight(cfg.baseURL.String(), "/") + "/rest/api/3/search/jql?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build jira request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuth(cfg.email, cfg.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("jira authentication failed: status=%d", resp.StatusCode)
	default:
		return nil, fmt.Errorf("jira request failed: status=%d", resp.StatusCode)
	}

	var payload struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode jira response: %w", err)
	}

	issues := make([]Issue, 0, len(payload.Issues))
	for _, it := range payload.Issues {
		key := strings.ToUpper(strings.TrimSpace(it.Key))
		if key == "" {
			continue
		}
		issues = append(issues, Issue{
			Key:       key,
			Summary:   strings.TrimSpace(it.Fields.Summary),
			TicketURL: strings.TrimRight(cfg.baseURL.String(), "/") + "/browse/" + url.PathEscape(key),
		})
	}
	return issues, nil
}

func (c *Client) ListScrumBoards(ctx context.Context) ([]Board, error) {
	return c.listScrumBoards(ctx, "")
}

func (c *Client) ListScrumBoardsByProject(ctx context.Context, projectKey string) ([]Board, error) {
	return c.listScrumBoards(ctx, strings.ToUpper(strings.TrimSpace(projectKey)))
}

func (c *Client) listScrumBoards(ctx context.Context, projectKey string) ([]Board, error) {
	cfg, err := loadEnvConfig()
	if err != nil {
		return nil, err
	}
	startAt := 0
	boards := make([]Board, 0, 32)
	for {
		q := url.Values{}
		q.Set("type", "scrum")
		q.Set("maxResults", "50")
		q.Set("startAt", fmt.Sprintf("%d", startAt))
		if strings.TrimSpace(projectKey) != "" {
			q.Set("projectKeyOrId", projectKey)
		}
		endpoint := strings.TrimRight(cfg.baseURL.String(), "/") + "/rest/agile/1.0/board?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("build jira request: %w", err)
		}
		req.Header.Set("Authorization", "Basic "+basicAuth(cfg.email, cfg.apiToken))
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("jira request failed: %w", err)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("jira authentication failed: status=%d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("jira request failed: status=%d", resp.StatusCode)
		}

		var payload struct {
			IsLast bool `json:"isLast"`
			Values []struct {
				ID       int    `json:"id"`
				Name     string `json:"name"`
				Type     string `json:"type"`
				Location struct {
					ProjectKey string `json:"projectKey"`
				} `json:"location"`
			} `json:"values"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("decode jira response: %w", err)
		}
		_ = resp.Body.Close()

		for _, v := range payload.Values {
			boards = append(boards, Board{
				ID:         v.ID,
				Name:       strings.TrimSpace(v.Name),
				Type:       strings.TrimSpace(v.Type),
				ProjectKey: strings.ToUpper(strings.TrimSpace(v.Location.ProjectKey)),
			})
		}
		if payload.IsLast || len(payload.Values) == 0 {
			break
		}
		startAt += len(payload.Values)
	}
	return boards, nil
}

func (c *Client) ListBoardSprintsActiveFuture(ctx context.Context, boardID int) ([]Sprint, error) {
	cfg, err := loadEnvConfig()
	if err != nil {
		return nil, err
	}
	if boardID <= 0 {
		return nil, fmt.Errorf("board id is required")
	}
	startAt := 0
	sprints := make([]Sprint, 0, 32)
	for {
		q := url.Values{}
		q.Set("state", "active,future")
		q.Set("maxResults", "50")
		q.Set("startAt", fmt.Sprintf("%d", startAt))
		endpoint := fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?%s", strings.TrimRight(cfg.baseURL.String(), "/"), boardID, q.Encode())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("build jira request: %w", err)
		}
		req.Header.Set("Authorization", "Basic "+basicAuth(cfg.email, cfg.apiToken))
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("jira request failed: %w", err)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("jira authentication failed: status=%d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("jira request failed: status=%d", resp.StatusCode)
		}

		var payload struct {
			IsLast bool `json:"isLast"`
			Values []struct {
				ID            int    `json:"id"`
				Name          string `json:"name"`
				State         string `json:"state"`
				OriginBoardID int    `json:"originBoardId"`
			} `json:"values"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("decode jira response: %w", err)
		}
		_ = resp.Body.Close()
		for _, v := range payload.Values {
			sprints = append(sprints, Sprint{
				ID:            v.ID,
				Name:          strings.TrimSpace(v.Name),
				State:         strings.TrimSpace(v.State),
				OriginBoardID: v.OriginBoardID,
			})
		}
		if payload.IsLast || len(payload.Values) == 0 {
			break
		}
		startAt += len(payload.Values)
	}
	return sprints, nil
}

func (c *Client) GetSprint(ctx context.Context, sprintID int) (Sprint, error) {
	cfg, err := loadEnvConfig()
	if err != nil {
		return Sprint{}, err
	}
	if sprintID <= 0 {
		return Sprint{}, fmt.Errorf("sprint id is required")
	}
	endpoint := fmt.Sprintf("%s/rest/agile/1.0/sprint/%d", strings.TrimRight(cfg.baseURL.String(), "/"), sprintID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Sprint{}, fmt.Errorf("build jira request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuth(cfg.email, cfg.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Sprint{}, fmt.Errorf("jira request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Sprint{}, fmt.Errorf("jira authentication failed: status=%d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return Sprint{}, fmt.Errorf("jira sprint not found: %d", sprintID)
	}
	if resp.StatusCode != http.StatusOK {
		return Sprint{}, fmt.Errorf("jira request failed: status=%d", resp.StatusCode)
	}

	var payload struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		State         string `json:"state"`
		OriginBoardID int    `json:"originBoardId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Sprint{}, fmt.Errorf("decode jira response: %w", err)
	}
	return Sprint{
		ID:            payload.ID,
		Name:          strings.TrimSpace(payload.Name),
		State:         strings.TrimSpace(payload.State),
		OriginBoardID: payload.OriginBoardID,
	}, nil
}

func (c *Client) ListProjectOpenSprints(ctx context.Context, projectKey string, maxResults int) ([]Sprint, error) {
	cfg, err := loadEnvConfig()
	if err != nil {
		return nil, err
	}
	key := strings.ToUpper(strings.TrimSpace(projectKey))
	if key == "" {
		return nil, fmt.Errorf("project key is required")
	}
	if maxResults <= 0 {
		maxResults = 100
	}

	jql := fmt.Sprintf("project = %s AND (sprint in openSprints() OR sprint in futureSprints()) ORDER BY Rank ASC", key)
	q := url.Values{}
	q.Set("jql", jql)
	q.Set("maxResults", fmt.Sprintf("%d", maxResults))
	q.Set("fields", "*all")
	endpoint := strings.TrimRight(cfg.baseURL.String(), "/") + "/rest/api/3/search/jql?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build jira request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuth(cfg.email, cfg.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("jira authentication failed: status=%d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira request failed: status=%d", resp.StatusCode)
	}

	var payload struct {
		Issues []struct {
			Fields map[string]json.RawMessage `json:"fields"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode jira response: %w", err)
	}

	out := make([]Sprint, 0, len(payload.Issues))
	seen := map[int]struct{}{}
	for _, issue := range payload.Issues {
		for _, raw := range issue.Fields {
			if sprints, ok := parseSprintFieldRaw(raw); ok {
				for _, s := range sprints {
					if s.ID <= 0 {
						continue
					}
					if _, exists := seen[s.ID]; exists {
						continue
					}
					seen[s.ID] = struct{}{}
					out = append(out, s)
				}
			}
		}
	}
	return out, nil
}

func parseSprintFieldRaw(raw json.RawMessage) ([]Sprint, bool) {
	type sprintPayload struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		State         string `json:"state"`
		OriginBoardID int    `json:"originBoardId"`
		RapidViewID   int    `json:"rapidViewId"`
	}
	toSprint := func(s sprintPayload) Sprint {
		boardID := s.OriginBoardID
		if boardID <= 0 {
			boardID = s.RapidViewID
		}
		return Sprint{
			ID:            s.ID,
			Name:          strings.TrimSpace(s.Name),
			State:         strings.TrimSpace(strings.ToLower(s.State)),
			OriginBoardID: boardID,
		}
	}
	var one sprintPayload
	if err := json.Unmarshal(raw, &one); err == nil && one.ID > 0 {
		return []Sprint{toSprint(one)}, true
	}
	var many []sprintPayload
	if err := json.Unmarshal(raw, &many); err == nil && len(many) > 0 {
		out := make([]Sprint, 0, len(many))
		for _, s := range many {
			if s.ID <= 0 {
				continue
			}
			out = append(out, toSprint(s))
		}
		return out, true
	}
	var oneLegacy string
	if err := json.Unmarshal(raw, &oneLegacy); err == nil {
		if sprint, ok := parseLegacySprintString(oneLegacy); ok {
			return []Sprint{sprint}, true
		}
	}
	var manyLegacy []string
	if err := json.Unmarshal(raw, &manyLegacy); err == nil && len(manyLegacy) > 0 {
		out := make([]Sprint, 0, len(manyLegacy))
		for _, v := range manyLegacy {
			sprint, ok := parseLegacySprintString(v)
			if !ok {
				continue
			}
			out = append(out, sprint)
		}
		return out, true
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err == nil {
		if out := parseSprintsFromAny(generic); len(out) > 0 {
			return out, true
		}
	}
	return nil, false
}

func parseSprintsFromAny(v any) []Sprint {
	switch t := v.(type) {
	case map[string]any:
		sprint, ok := parseSprintFromMap(t)
		if !ok {
			return nil
		}
		return []Sprint{sprint}
	case []any:
		out := make([]Sprint, 0, len(t))
		for _, item := range t {
			out = append(out, parseSprintsFromAny(item)...)
		}
		return out
	case string:
		sprint, ok := parseLegacySprintString(t)
		if !ok {
			return nil
		}
		return []Sprint{sprint}
	default:
		return nil
	}
}

func parseSprintFromMap(m map[string]any) (Sprint, bool) {
	id := parseAnyInt(m["id"])
	if id <= 0 {
		return Sprint{}, false
	}
	boardID := parseAnyInt(m["originBoardId"])
	if boardID <= 0 {
		boardID = parseAnyInt(m["rapidViewId"])
	}
	if boardID <= 0 {
		boardID = parseAnyInt(m["boardId"])
	}
	return Sprint{
		ID:            id,
		Name:          strings.TrimSpace(parseAnyString(m["name"])),
		State:         strings.TrimSpace(strings.ToLower(parseAnyString(m["state"]))),
		OriginBoardID: boardID,
	}, true
}

func parseAnyInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

func parseAnyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return ""
	}
}

func parseLegacySprintString(v string) (Sprint, bool) {
	raw := strings.TrimSpace(v)
	if raw == "" {
		return Sprint{}, false
	}
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return Sprint{}, false
	}
	inner := raw[start+1 : end]
	matches := legacySprintKVPattern.FindAllStringSubmatch(inner, -1)
	if len(matches) == 0 {
		return Sprint{}, false
	}
	var sprint Sprint
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		key := strings.TrimSpace(m[1])
		val := strings.TrimSpace(m[2])
		switch key {
		case "id":
			id, err := strconv.Atoi(val)
			if err == nil {
				sprint.ID = id
			}
		case "name":
			sprint.Name = val
		case "state":
			sprint.State = strings.ToLower(val)
		case "originBoardId", "rapidViewId":
			boardID, err := strconv.Atoi(val)
			if err == nil && boardID > 0 {
				sprint.OriginBoardID = boardID
			}
		}
	}
	if sprint.ID <= 0 {
		return Sprint{}, false
	}
	sprint.Name = strings.TrimSpace(sprint.Name)
	sprint.State = strings.TrimSpace(sprint.State)
	return sprint, true
}

func (c *Client) ListBoardProjectKeys(ctx context.Context, boardID int) ([]string, error) {
	cfg, err := loadEnvConfig()
	if err != nil {
		return nil, err
	}
	if boardID <= 0 {
		return nil, fmt.Errorf("board id is required")
	}
	endpoint := fmt.Sprintf("%s/rest/agile/1.0/board/%d/project", strings.TrimRight(cfg.baseURL.String(), "/"), boardID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build jira request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuth(cfg.email, cfg.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("jira authentication failed: status=%d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("jira board not found: %d", boardID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira request failed: status=%d", resp.StatusCode)
	}

	var payload struct {
		Values []struct {
			Key string `json:"key"`
		} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode jira response: %w", err)
	}
	keys := make([]string, 0, len(payload.Values))
	for _, v := range payload.Values {
		key := strings.ToUpper(strings.TrimSpace(v.Key))
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	return keys, nil
}
