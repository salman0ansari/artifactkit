package artifactkit

import (
	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
	"github.com/salman0ansari/artifactkit/resource"
)

// Option configures an Engine.
type Option func(*Engine)

func WithLimits(limits artifact.Limits) Option {
	return func(engine *Engine) { engine.limits = limits }
}

// WithResourceStore supplies a store shared by one or more engines. A nil store disables lazy reads.
func WithResourceStore(store *resource.Store) Option {
	return func(engine *Engine) {
		engine.resources = store
		engine.resourceStoreConfigured = true
	}
}

func WithRegistry(registry *parser.Registry) Option {
	return func(engine *Engine) {
		if registry != nil {
			engine.registry = registry
		}
	}
}
