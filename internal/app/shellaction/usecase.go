package shellaction

import "fmt"

type Port interface {
	WriteActionLine(line string) error
}

type Service struct {
	port Port
}

func NewService(port Port) *Service {
	return &Service{port: port}
}

func (s *Service) EmitCD(path string) error {
	line := fmt.Sprintf("cd %s\n", shellSingleQuote(path))
	return s.port.WriteActionLine(line)
}

func shellSingleQuote(s string) string {
	if s == "" {
		return "''"
	}
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'"'"'`
			continue
		}
		out += string(r)
	}
	out += "'"
	return out
}
