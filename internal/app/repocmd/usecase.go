package repocmd

import (
	"context"
)

type Request struct {
	CWD           string
	DebugTag      string
	RequireGit    bool
	TouchRegistry bool
}

type Session struct {
	Root         string
	RepoPoolPath string
}

type Port interface {
	EnsureGitInPath() error
	ResolveRoot(cwd string) (string, error)
	EnsureDebugLog(root string, tag string) error
	ResolveRepoPoolPath() (string, error)
	TouchRegistry(root string) error
}

type Service struct {
	port Port
}

func NewService(port Port) *Service {
	return &Service{port: port}
}

func (s *Service) Run(ctx context.Context, req Request) (Session, error) {
	if req.RequireGit {
		if err := s.port.EnsureGitInPath(); err != nil {
			return Session{}, err
		}
	}

	root, err := s.port.ResolveRoot(req.CWD)
	if err != nil {
		return Session{}, err
	}
	if req.DebugTag != "" {
		if err := s.port.EnsureDebugLog(root, req.DebugTag); err != nil {
			return Session{}, err
		}
	}

	repoPoolPath, err := s.port.ResolveRepoPoolPath()
	if err != nil {
		return Session{}, err
	}
	if req.TouchRegistry {
		if err := s.port.TouchRegistry(root); err != nil {
			return Session{}, err
		}
	}

	return Session{
		Root:         root,
		RepoPoolPath: repoPoolPath,
	}, nil
}
