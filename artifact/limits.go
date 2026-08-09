package artifact

import "fmt"

// Limits bounds parser work and protects agent runtimes from hostile artifacts.
type Limits struct {
	MaxInputBytes       int64
	MaxTextBytes        int64
	MaxNodes            int
	MaxNestingDepth     int
	MaxArchiveDepth     int
	MaxArchiveEntries   int
	MaxExpandedBytes    int64
	MaxCompressionRatio float64
}

// DefaultLimits returns conservative limits suitable for local agent tools.
func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes:       256 << 20,
		MaxTextBytes:        32 << 20,
		MaxNodes:            100_000,
		MaxNestingDepth:     256,
		MaxArchiveDepth:     4,
		MaxArchiveEntries:   10_000,
		MaxExpandedBytes:    512 << 20,
		MaxCompressionRatio: 200,
	}
}

// Validate rejects nonsensical or disabled safety limits.
func (l Limits) Validate() error {
	if l.MaxInputBytes <= 0 || l.MaxTextBytes <= 0 || l.MaxNodes <= 0 || l.MaxNestingDepth <= 0 {
		return fmt.Errorf("artifactkit: core limits must be positive")
	}
	if l.MaxArchiveDepth <= 0 || l.MaxArchiveEntries <= 0 || l.MaxExpandedBytes <= 0 || l.MaxCompressionRatio <= 0 {
		return fmt.Errorf("artifactkit: archive limits must be positive")
	}
	return nil
}

// LimitError identifies a specific safety limit that was exceeded.
type LimitError struct {
	Limit string
	Value int64
	Max   int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("artifactkit: %s limit exceeded: %d > %d", e.Limit, e.Value, e.Max)
}
