// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// This package provides a means to copy over comments between
// two kyaml/yaml.RNode trees. This code is derived from
// the sigs.k8s.io/kustomize/kyaml/comments package, at revision
// 600d4f2c0bf174abd76d03e49939ee0c34b2a019.
//
// It has been slightly modified and adapted to not lose any
// comment from the old tree, although the node the comment is
// attached to doesn't exist in the new tree. To solve this,
// this package moves any such comments to the beginning of the
// file.
// This file is a temporary means as long as we're waiting for
// these code changes to get upstreamed to its origin, the kustomize repo.
// https://pkg.go.dev/sigs.k8s.io/kustomize/kyaml/comments?tab=doc#CopyComments

package comments

import (
	"fmt"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/kyaml/yaml/walk"
)

// CopyComments recursively copies the comments on fields in from to fields in to
func CopyComments(from, to *yaml.RNode, moveCommentsTop bool) error {
	// create the copier struct for the specified mode
	c := &copier{moveCommentsTop, nil}

	// copy over comments for the root tree(s)
	c.copyFieldComments(from, to)

	// walk the fields copying comments
	_, err := walk.Walker{
		Sources:            []*yaml.RNode{from, to},
		Visitor:            c,
		VisitKeysAsScalars: true}.Walk()

	// restore lost comments to the top of the document, if applicable
	if moveCommentsTop {
		c.restoreLostComments(to)
	}

	return err
}

// copier implements walk.Visitor, and copies comments to fields shared between 2 instances
// of a resource
type copier struct {
	// moveCommentsTop specifies whether to recover lost comments or not
	moveCommentsTop bool
	// if moveCommentsTop is true, this slice will be populated with lost comment entries while iterating
	lostComments []lostComment
}

// lostComment specifies a mapping between a fieldName (in the old structure) which doesn't exist in the
// new tree, and its related comment
type lostComment struct {
	fieldName string
	comment   string
}

func (c *copier) VisitMap(s walk.Sources, _ *openapi.ResourceSchema) (*yaml.RNode, error) {
	c.copyFieldComments(s.Dest(), s.Origin())
	return s.Dest(), nil
}

func (c *copier) VisitScalar(s walk.Sources, _ *openapi.ResourceSchema) (*yaml.RNode, error) {
	to := s.Origin()
	// TODO: File a bug with upstream yaml to handle comments for FoldedStyle scalar nodes
	// Hack: convert FoldedStyle scalar node to DoubleQuotedStyle as the line comments are
	// being serialized without space
	// https://github.com/GoogleContainerTools/kpt/issues/766
	if to != nil && to.Document().Style == yaml.FoldedStyle {
		to.Document().Style = yaml.DoubleQuotedStyle
	}

	c.copyFieldComments(s.Dest(), to)
	return s.Dest(), nil
}

func (c *copier) VisitList(s walk.Sources, _ *openapi.ResourceSchema, _ walk.ListKind) (*yaml.RNode, error) {
	c.copyFieldComments(s.Dest(), s.Origin())
	destItems := s.Dest().Content()
	originItems := s.Origin().Content()

	for i := 0; i < len(destItems) && i < len(originItems); i++ {
		dest := destItems[i]
		origin := originItems[i]

		if dest.Value == origin.Value {
			c.copyFieldComments(yaml.NewRNode(dest), yaml.NewRNode(origin))
		}
	}

	return s.Dest(), nil
}

// copyFieldComments copies the comment from one field to another
func (c *copier) copyFieldComments(from, to *yaml.RNode) {
	// If either from or to doesn't exist, return quickly
	if from == nil || to == nil {

		// If we asked for moving lost comments (i.e. if from is non-nil and to is nil),
		// do it through the moveLostCommentToTop function
		if c.moveCommentsTop && from != nil && to == nil {
			c.rememberLostComments(from)
		}
		return
	}

	if to.Document().LineComment == "" {
		to.Document().LineComment = from.Document().LineComment
	}
	if to.Document().HeadComment == "" {
		to.Document().HeadComment = from.Document().HeadComment
	}
	if to.Document().FootComment == "" {
		to.Document().FootComment = from.Document().FootComment
	}
}

// rememberLostComments goes through the comments attached to the from node
// and adds them to the internal lostComments slice for usage after the tree
// walk
func (c *copier) rememberLostComments(from *yaml.RNode) {
	fromName := from.Document().Value
	fComments := []string{
		from.Document().LineComment,
		from.Document().HeadComment,
		from.Document().FootComment,
	}
	for _, cStr := range fComments {
		cStr = strings.TrimPrefix(cStr, "#")
		cStr = strings.TrimSuffix(cStr, "\n")
		cStr = strings.TrimSpace(cStr)

		if cStr != "" {
			c.lostComments = append(c.lostComments, lostComment{
				fieldName: fromName,
				comment:   cStr,
			})
		}
	}
}

// restoreLostComments writes the cached lost comments to the top of the to
// YAML tree
func (c *copier) restoreLostComments(to *yaml.RNode) {
	for i, c := range c.lostComments {
		if i == 0 {
			to.Document().HeadComment += "\nComments lost during file manipulation:"
		}
		to.Document().HeadComment += fmt.Sprintf("\n# Field name %q: %q", c.fieldName, c.comment)
	}
	to.Document().HeadComment = strings.TrimPrefix(to.Document().HeadComment, "\n")
}
