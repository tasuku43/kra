package appports

import (
	"context"
	"fmt"

	"github.com/tasuku43/gionx/internal/app/wsimport"
	"github.com/tasuku43/gionx/internal/infra/jira"
)

type WSImportJiraPort struct {
	client *jira.Client
}

func NewWSImportJiraPort() *WSImportJiraPort {
	return &WSImportJiraPort{client: jira.NewClient()}
}

func (p *WSImportJiraPort) SearchIssuesByJQL(ctx context.Context, jql string, maxResults int) ([]wsimport.JiraIssue, error) {
	issues, err := p.client.SearchIssuesByJQL(ctx, jql, maxResults)
	if err != nil {
		return nil, fmt.Errorf("search jira issues: %w", err)
	}
	out := make([]wsimport.JiraIssue, 0, len(issues))
	for _, it := range issues {
		out = append(out, wsimport.JiraIssue{
			Key:       it.Key,
			Summary:   it.Summary,
			TicketURL: it.TicketURL,
		})
	}
	return out, nil
}

func (p *WSImportJiraPort) ListScrumBoards(ctx context.Context) ([]wsimport.JiraBoard, error) {
	boards, err := p.client.ListScrumBoards(ctx)
	if err != nil {
		return nil, fmt.Errorf("list jira boards: %w", err)
	}
	out := make([]wsimport.JiraBoard, 0, len(boards))
	for _, b := range boards {
		out = append(out, wsimport.JiraBoard{ID: b.ID, Name: b.Name, ProjectKey: b.ProjectKey})
	}
	return out, nil
}

func (p *WSImportJiraPort) ListScrumBoardsByProject(ctx context.Context, projectKey string) ([]wsimport.JiraBoard, error) {
	boards, err := p.client.ListScrumBoardsByProject(ctx, projectKey)
	if err != nil {
		return nil, fmt.Errorf("list jira boards by project: %w", err)
	}
	out := make([]wsimport.JiraBoard, 0, len(boards))
	for _, b := range boards {
		out = append(out, wsimport.JiraBoard{ID: b.ID, Name: b.Name, ProjectKey: b.ProjectKey})
	}
	return out, nil
}

func (p *WSImportJiraPort) ListBoardSprintsActiveFuture(ctx context.Context, boardID int) ([]wsimport.JiraSprint, error) {
	sprints, err := p.client.ListBoardSprintsActiveFuture(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("list jira sprints: %w", err)
	}
	out := make([]wsimport.JiraSprint, 0, len(sprints))
	for _, s := range sprints {
		out = append(out, wsimport.JiraSprint{
			ID:            s.ID,
			Name:          s.Name,
			State:         s.State,
			OriginBoardID: s.OriginBoardID,
		})
	}
	return out, nil
}

func (p *WSImportJiraPort) GetSprint(ctx context.Context, sprintID int) (wsimport.JiraSprint, error) {
	s, err := p.client.GetSprint(ctx, sprintID)
	if err != nil {
		return wsimport.JiraSprint{}, fmt.Errorf("get jira sprint: %w", err)
	}
	return wsimport.JiraSprint{
		ID:            s.ID,
		Name:          s.Name,
		State:         s.State,
		OriginBoardID: s.OriginBoardID,
	}, nil
}

func (p *WSImportJiraPort) ListBoardProjectKeys(ctx context.Context, boardID int) ([]string, error) {
	keys, err := p.client.ListBoardProjectKeys(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("list jira board projects: %w", err)
	}
	return keys, nil
}

func (p *WSImportJiraPort) ListProjectOpenSprints(ctx context.Context, projectKey string, maxResults int) ([]wsimport.JiraSprint, error) {
	sprints, err := p.client.ListProjectOpenSprints(ctx, projectKey, maxResults)
	if err != nil {
		return nil, fmt.Errorf("list jira open sprints by project: %w", err)
	}
	out := make([]wsimport.JiraSprint, 0, len(sprints))
	for _, s := range sprints {
		out = append(out, wsimport.JiraSprint{
			ID:            s.ID,
			Name:          s.Name,
			State:         s.State,
			OriginBoardID: s.OriginBoardID,
		})
	}
	return out, nil
}
