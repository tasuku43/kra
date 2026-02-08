package repocmd

import (
	"context"
	"database/sql"
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
	DB           *sql.DB
}

type Port interface {
	EnsureGitInPath() error
	ResolveRoot(cwd string) (string, error)
	EnsureDebugLog(root string, tag string) error
	ResolveStateDBPath(root string) (string, error)
	ResolveRepoPoolPath() (string, error)
	OpenState(ctx context.Context, dbPath string) (*sql.DB, error)
	EnsureSettings(ctx context.Context, db *sql.DB, root string, repoPoolPath string) error
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

	dbPath, err := s.port.ResolveStateDBPath(root)
	if err != nil {
		return Session{}, err
	}
	repoPoolPath, err := s.port.ResolveRepoPoolPath()
	if err != nil {
		return Session{}, err
	}
	db, err := s.port.OpenState(ctx, dbPath)
	if err != nil {
		return Session{}, err
	}
	if err := s.port.EnsureSettings(ctx, db, root, repoPoolPath); err != nil {
		_ = db.Close()
		return Session{}, err
	}
	if req.TouchRegistry {
		if err := s.port.TouchRegistry(root); err != nil {
			_ = db.Close()
			return Session{}, err
		}
	}

	return Session{
		Root:         root,
		RepoPoolPath: repoPoolPath,
		DB:           db,
	}, nil
}
