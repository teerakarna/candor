package gitops

import (
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// setYAMLPath replaces the scalar value at the dot-separated path within a YAML document,
// preserving everything else in the document (comments, key order, quoting style) exactly - it
// mutates one node in the parsed tree and re-serializes, rather than round-tripping through a
// generic map, which would lose formatting a real GitOps repo's maintainers rely on.
func setYAMLPath(data []byte, path, value string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("empty yaml document")
	}

	node, err := findScalar(doc.Content[0], strings.Split(path, "."))
	if err != nil {
		return nil, err
	}
	node.Value = value

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("marshaling yaml: %w", err)
	}
	return out, nil
}

// findScalar walks a mapping node by dot-path segments and returns the scalar node at the end.
func findScalar(node *yaml.Node, keys []string) (*yaml.Node, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a yaml mapping while resolving path %q, found kind %d", strings.Join(keys, "."), node.Kind)
	}

	key := keys[0]
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != key {
			continue
		}
		value := node.Content[i+1]
		if len(keys) == 1 {
			if value.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("yaml path %q does not resolve to a scalar value", key)
			}
			return value, nil
		}
		return findScalar(value, keys[1:])
	}
	return nil, fmt.Errorf("yaml path segment %q not found", key)
}
