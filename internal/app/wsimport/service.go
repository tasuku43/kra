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

type Service struct {
	jiraPort JiraIssueListPort
}

func NewService(jiraPort JiraIssueListPort) *Service {
	return &Service{jiraPort: jiraPort}
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
