package comments

import (
	"fmt"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// lostComment specifies a mapping between a fieldName (in the old structure), which doesn't exist in the
// new tree, and its related comment. It optionally specifies the line number of the comment, a positive
// line number is used to distinguish inline comments, which require special handling to resolve the
// correct field name, since they are attached to the value and not the key of a YAML key-value pair.
type lostComment struct {
	fieldName string
	comment   string
	line      int
}

// Since the YAML walker needs to visit all keys as scalar nodes, we have no way of distinguishing keys from
// values when trying to resolve the field names for inline comments. By tracking the leftmost key (lowest
// column value, be it a key or value) for each row, we can figure out the actual key for inline comments
// and not accidentally use a value as the field name, since keys are guaranteed to come before values.
type trackedKey struct {
	name   string
	column int
}

// trackKey compares the column position of the given node to the stored best (lowest) column position for the
// node's line and replaces the best if the given node is more likely to be a key (has a smaller column value).
func (c *copier) trackKey(node *yaml.Node) {
	// If the given key doesn't have a smaller column value, return.
	if key, ok := c.trackedKeys[node.Line]; ok {
		if key.column < node.Column {
			return
		}
	}

	// Store the new best tracked key for the line.
	c.trackedKeys[node.Line] = trackedKey{
		name:   node.Value,
		column: node.Column,
	}
}

// parseComments parses the line, head and foot comments of the given node in this
// order and cleans them up (removes the potential "#" prefix and trims whitespace).
func parseComments(node *yaml.Node) (comments []string) {
	for _, comment := range []string{node.LineComment, node.HeadComment, node.FootComment} {
		comments = append(comments, strings.TrimSpace(strings.TrimPrefix(comment, "#")))
	}

	return
}

// rememberLostComments goes through the comments attached to the 'from' node and adds
// them to the internal lostComments slice for usage after the tree walk. It also
// stores the line numbers for inline comments for resolving the correct field names.
func (c *copier) rememberLostComments(from *yaml.RNode) {
	// Track the given node as a potential key for inline comments.
	c.trackKey(from.Document())

	// Get the field name, for head/foot comments this is the correct key,
	// but for inline comments this resolves to the value of the field instead.
	fieldName := from.Document().Value
	comments := parseComments(from.Document())
	line := -1 // Don't store the line number of the comment by default, this is reserved for inline comments.

	for i, comment := range comments {
		// If the line number is set (positive), an inline comment
		// has been registered for this node and we can stop parsing.
		if line >= 0 {
			break
		}

		// Do not store blank comment entries (nonexistent comments).
		if len(comment) == 0 {
			continue
		}

		if i == 0 {
			// If this node has an inline comment, store its line
			// number for resolving the correct field name later.
			line = from.Document().Line
		}

		// Append the lost comment to the slice of copier.
		c.lostComments = append(c.lostComments, lostComment{
			fieldName: fieldName,
			comment:   comment,
			line:      line,
		})
	}
}

// restoreLostComments writes the cached lost comments to the top of the to YAML tree.
// If it encounters inline comments, it will check the cached tracked keys for the
// best key for the line on which the comment resided. If no key is found for some
// reason, it will use the stored field name (the field value) as the key.
func (c *copier) restoreLostComments(to *yaml.RNode) {
	for i, lc := range c.lostComments {
		if i == 0 {
			to.Document().HeadComment += "\nComments lost during file manipulation:"
		}

		fieldName := lc.fieldName
		if lc.line >= 0 {
			// This is an inline comment, resolve the field name from the tracked keys.
			if key, ok := c.trackedKeys[lc.line]; ok {
				fieldName = key.name
			}
		}

		to.Document().HeadComment += fmt.Sprintf("\n# Field %q: %q", fieldName, lc.comment)
	}

	to.Document().HeadComment = strings.TrimPrefix(to.Document().HeadComment, "\n")
}
