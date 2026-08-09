package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Finalize assigns content and node IDs and verifies normalized output limits.
func Finalize(a *Artifact, source []byte, limits Limits) error {
	if a == nil {
		return fmt.Errorf("artifactkit: parser returned a nil artifact")
	}
	if err := limits.Validate(); err != nil {
		return err
	}

	digest := sha256.Sum256(source)
	a.Digest = hex.EncodeToString(digest[:])
	a.ID = "sha256:" + a.Digest
	a.Size = int64(len(source))
	if a.Root.Kind == "" {
		a.Root.Kind = KindDocument
	}

	var nodes int64
	var textBytes int64
	var assign func(*Node, string) error
	assign = func(n *Node, path string) error {
		nodes++
		if nodes > int64(limits.MaxNodes) {
			return &LimitError{Limit: "nodes", Value: nodes, Max: int64(limits.MaxNodes)}
		}
		textBytes += int64(len(n.Text))
		if textBytes > limits.MaxTextBytes {
			return &LimitError{Limit: "text bytes", Value: textBytes, Max: limits.MaxTextBytes}
		}
		nodeDigest := sha256.Sum256([]byte(a.Digest + "\x00" + string(n.Kind) + "\x00" + path))
		n.ID = "node_" + hex.EncodeToString(nodeDigest[:10])
		for i := range n.Children {
			if err := assign(&n.Children[i], fmt.Sprintf("%s/%d", path, i)); err != nil {
				return err
			}
		}
		return nil
	}
	return assign(&a.Root, "root")
}
