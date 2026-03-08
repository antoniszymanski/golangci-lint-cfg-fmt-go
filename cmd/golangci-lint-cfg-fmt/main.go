// SPDX-FileCopyrightText: 2026 Antoni Szymański
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

type Cli struct {
	Path string `arg:"" type:"path"`
}

func main() {
	var cli Cli
	ctx := kong.Parse(&cli,
		kong.Name("golangci-lint-cfg-fmt"),
		kong.Description("A formatter for golangci-lint YAML config files"),
		kong.UsageOnError(),
	)
	ctx.FatalIfErrorf(ctx.Run())
}

func (c *Cli) Run() error {
	f := os.Stdin
	if c.Path != "-" {
		var err error
		f, err = os.OpenFile(c.Path, os.O_RDWR, 0600)
		if err != nil {
			return fmt.Errorf("failed to open the file: %w", err)
		}
		defer f.Close() //nolint:errcheck
	}
	in, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read from the file: %w", err)
	}
	file, err := parser.ParseBytes(in, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file as YAML: %w", err)
	}
	if len(file.Docs) != 1 {
		return fmt.Errorf("exactly one document was expected in the YAML file, but %d were found", len(file.Docs))
	}
	rootNode := file.Docs[0].Body
	root, ok := rootNode.(*ast.MappingNode)
	if !ok {
		return fmt.Errorf("the root node is not a mapping node, but %T", rootNode)
	}
	lintersNode := get(root, "linters")
	if lintersNode == nil {
		return errors.New("key 'linters' not found in the root node")
	}
	linters, ok := lintersNode.(*ast.MappingNode)
	if !ok {
		return fmt.Errorf("the 'linters' node is not a mapping node, but %T", lintersNode)
	}
	disableNode := get(linters, "disable")
	if disableNode == nil {
		return errors.New("key 'disable' not found in the 'linters' node")
	}
	disable, ok := disableNode.(*ast.SequenceNode)
	if !ok {
		return fmt.Errorf("the 'disable' node is not a sequence node, but %T", disableNode)
	}

	stringNodes := make([]*ast.StringNode, 0, len(disable.Values))
	for i, value := range disable.Values {
		stringNode, ok := value.(*ast.StringNode)
		if !ok {
			return fmt.Errorf("the node at index %d is not a string node, but %T", i, value)
		}
		stringNodes = append(stringNodes, stringNode)
	}
	slices.SortStableFunc(stringNodes, func(a, b *ast.StringNode) int {
		aHasComment := a.Comment != nil
		bHasComment := b.Comment != nil
		switch {
		case aHasComment == bHasComment:
			return strings.Compare(a.Value, b.Value)
		case !aHasComment && bHasComment:
			return -1
		default:
			return 1
		}
	})
	disable.Values = disable.Values[:0]
	for _, stringNode := range stringNodes {
		disable.Values = append(disable.Values, stringNode)
	}

	out, err := root.MarshalYAML()
	if err != nil {
		return fmt.Errorf("failed to encode to a YAML text: %w", err)
	}
	out = append(out, '\n')
	if c.Path == "-" {
		f = os.Stdout
	} else {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("failed to set the file offset: %w", err)
		}
		if err := f.Truncate(0); err != nil {
			return fmt.Errorf("failed to truncate the file: %w", err)
		}
	}
	if _, err := f.Write(out); err != nil {
		return fmt.Errorf("failed to write to the file: %w", err)
	}
	return nil
}

func get(n *ast.MappingNode, key string) ast.Node {
	var value ast.Node
	for _, keyvalue := range n.Values {
		if key == keyvalue.Key.String() {
			value = keyvalue.Value
			break
		}
	}
	return value
}
