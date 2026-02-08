package cli

import (
	"time"

	"github.com/tasuku43/gionx/internal/stateregistry"
)

func (c *CLI) touchStateRegistry(root string) error {
	return stateregistry.Touch(root, time.Now())
}
