package wsimport

import (
	"context"
	"fmt"
	"strings"
)

type JiraIssue struct {
	Key       string
	Summary   string
	TicketURL string
}

type GitHubScope struct {
	Kind  string
	Value string
}

type GitHubIssue struct {
	Repository string
	Number     int
	Title      string
	URL        string
}

type GitHubPullRequest struct {
	Repository string
	Number     int
	Title      string
	URL        string
	IsDraft    bool
	HeadRef    string
}

type JiraBoard struct {
	ID         int
	Name       string
	ProjectKey string
}

type JiraSprint struct {
	ID            int
	Name          string
	State         string
	OriginBoardID int
}

type JiraIssueListPort interface {
	SearchIssuesByJQL(ctx context.Context, jql string, maxResults int) ([]JiraIssue, error)
	ListScrumBoards(ctx context.Context) ([]JiraBoard, error)
	ListScrumBoardsByProject(ctx context.Context, projectKey string) ([]JiraBoard, error)
	ListBoardSprintsActiveFuture(ctx context.Context, boardID int) ([]JiraSprint, error)
	GetSprint(ctx context.Context, sprintID int) (JiraSprint, error)
	ListBoardProjectKeys(ctx context.Context, boardID int) ([]string, error)
	ListProjectOpenSprints(ctx context.Context, projectKey string, maxResults int) ([]JiraSprint, error)
}

type WorkspaceInput struct {
	ID        string
	Title     string
	SourceURL string
}

type GitHubWorkspaceInput struct {
	ID         string
	Title      string
	SourceURL  string
	Repository string
	Number     int
	Branch     string
}

type GitHubImportPort interface {
	CheckAuth(ctx context.Context) error
	SearchIssues(ctx context.Context, scope GitHubScope, state string, maxResults int) ([]GitHubIssue, error)
	SearchReviewPullRequests(ctx context.Context, scope GitHubScope, maxResults int) ([]GitHubPullRequest, error)
}

type Service struct {
	jiraPort   JiraIssueListPort
	githubPort GitHubImportPort
}

func NewService(jiraPort JiraIssueListPort, githubPort GitHubImportPort) *Service {
	return &Service{jiraPort: jiraPort, githubPort: githubPort}
}

func (s *Service) ResolveWorkspaceInputsByJQL(ctx context.Context, jql string, maxResults int) ([]WorkspaceInput, error) {
	if s.jiraPort == nil {
		return nil, fmt.Errorf("jira issue list port is not configured")
	}
	issues, err := s.jiraPort.SearchIssuesByJQL(ctx, jql, maxResults)
	if err != nil {
		return nil, err
	}
	inputs := make([]WorkspaceInput, 0, len(issues))
	for _, issue := range issues {
		key := strings.TrimSpace(issue.Key)
		if key == "" {
			continue
		}
		inputs = append(inputs, WorkspaceInput{
			ID:        key,
			Title:     strings.TrimSpace(issue.Summary),
			SourceURL: strings.TrimSpace(issue.TicketURL),
		})
	}
	return inputs, nil
}

func (s *Service) ListScrumBoards(ctx context.Context) ([]JiraBoard, error) {
	if s.jiraPort == nil {
		return nil, fmt.Errorf("jira issue list port is not configured")
	}
	return s.jiraPort.ListScrumBoards(ctx)
}

func (s *Service) ListScrumBoardsByProject(ctx context.Context, projectKey string) ([]JiraBoard, error) {
	if s.jiraPort == nil {
		return nil, fmt.Errorf("jira issue list port is not configured")
	}
	return s.jiraPort.ListScrumBoardsByProject(ctx, projectKey)
}

func (s *Service) ListBoardSprintsActiveFuture(ctx context.Context, boardID int) ([]JiraSprint, error) {
	if s.jiraPort == nil {
		return nil, fmt.Errorf("jira issue list port is not configured")
	}
	return s.jiraPort.ListBoardSprintsActiveFuture(ctx, boardID)
}

func (s *Service) GetSprint(ctx context.Context, sprintID int) (JiraSprint, error) {
	if s.jiraPort == nil {
		return JiraSprint{}, fmt.Errorf("jira issue list port is not configured")
	}
	return s.jiraPort.GetSprint(ctx, sprintID)
}

func (s *Service) ListBoardProjectKeys(ctx context.Context, boardID int) ([]string, error) {
	if s.jiraPort == nil {
		return nil, fmt.Errorf("jira issue list port is not configured")
	}
	return s.jiraPort.ListBoardProjectKeys(ctx, boardID)
}

func (s *Service) ListProjectOpenSprints(ctx context.Context, projectKey string, maxResults int) ([]JiraSprint, error) {
	if s.jiraPort == nil {
		return nil, fmt.Errorf("jira issue list port is not configured")
	}
	return s.jiraPort.ListProjectOpenSprints(ctx, projectKey, maxResults)
}

func (s *Service) ResolveGitHubIssueWorkspaceInputs(ctx context.Context, scope GitHubScope, state string, maxResults int) ([]GitHubWorkspaceInput, error) {
	if s.githubPort == nil {
		return nil, fmt.Errorf("github import port is not configured")
	}
	if err := s.githubPort.CheckAuth(ctx); err != nil {
		return nil, err
	}
	items, err := s.githubPort.SearchIssues(ctx, scope, state, maxResults)
	if err != nil {
		return nil, err
	}
	inputs := make([]GitHubWorkspaceInput, 0, len(items))
	for _, item := range items {
		repo := strings.TrimSpace(item.Repository)
		if repo == "" || item.Number <= 0 {
			continue
		}
		inputs = append(inputs, GitHubWorkspaceInput{
			ID:         buildGitHubWorkspaceID(repo, "issue", item.Number),
			Title:      strings.TrimSpace(item.Title),
			SourceURL:  strings.TrimSpace(item.URL),
			Repository: repo,
			Number:     item.Number,
		})
	}
	return inputs, nil
}

func (s *Service) ResolveGitHubReviewWorkspaceInputs(ctx context.Context, scope GitHubScope, maxResults int) ([]GitHubWorkspaceInput, error) {
	if s.githubPort == nil {
		return nil, fmt.Errorf("github import port is not configured")
	}
	if err := s.githubPort.CheckAuth(ctx); err != nil {
		return nil, err
	}
	items, err := s.githubPort.SearchReviewPullRequests(ctx, scope, maxResults)
	if err != nil {
		return nil, err
	}
	inputs := make([]GitHubWorkspaceInput, 0, len(items))
	for _, item := range items {
		repo := strings.TrimSpace(item.Repository)
		if repo == "" || item.Number <= 0 {
			continue
		}
		inputs = append(inputs, GitHubWorkspaceInput{
			ID:         buildGitHubWorkspaceID(repo, "pr", item.Number),
			Title:      strings.TrimSpace(item.Title),
			SourceURL:  strings.TrimSpace(item.URL),
			Repository: repo,
			Number:     item.Number,
			Branch:     strings.TrimSpace(item.HeadRef),
		})
	}
	return inputs, nil
}

func buildGitHubWorkspaceID(repo string, kind string, number int) string {
	replacer := strings.NewReplacer("/", "-", "_", "-", ".", "-", " ", "-")
	normalizedRepo := strings.ToLower(strings.Trim(strings.ReplaceAll(replacer.Replace(strings.TrimSpace(repo)), "--", "-"), "-"))
	return fmt.Sprintf("%s-%s-%d", normalizedRepo, strings.TrimSpace(kind), number)
}
