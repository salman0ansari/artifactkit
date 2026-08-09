package artifactkit

import (
	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

// Option configures an Engine.
type Option func(*Engine)

func WithLimits(limits artifact.Limits) Option {
	return func(engine *Engine) { engine.limits = limits }
}

func WithRegistry(registry *parser.Registry) Option {
	return func(engine *Engine) {
		if registry != nil {
			engine.registry = registry
		}
	}
}
