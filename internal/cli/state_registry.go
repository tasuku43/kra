package cli

import (
	"time"

	"github.com/tasuku43/gionx/internal/stateregistry"
)

func (c *CLI) touchStateRegistry(root string, stateDBPath string) error {
	return stateregistry.Touch(root, stateDBPath, time.Now())
}
