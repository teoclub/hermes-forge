package plugin

import (
	"context"

	"github.com/teoclub/hermes-forge/internal/engine"
)

type Plugin interface {
	Name() string
	Start(ctx context.Context, eng *engine.AgentEngine) error
}
