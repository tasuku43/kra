package initcmd

import "context"

type Request struct {
	Root string
}

type Result struct {
	Root string
}

type Port interface {
	EnsureLayout(root string) error
	TouchRegistry(root string) error
}

type Service struct {
	port Port
}

func NewService(port Port) *Service {
	return &Service{port: port}
}

func (s *Service) Run(ctx context.Context, req Request) (Result, error) {
	if err := s.port.EnsureLayout(req.Root); err != nil {
		return Result{}, err
	}
	if err := s.port.TouchRegistry(req.Root); err != nil {
		return Result{}, err
	}
	return Result{Root: req.Root}, nil
}
