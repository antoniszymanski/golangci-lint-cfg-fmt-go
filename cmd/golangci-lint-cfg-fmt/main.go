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
	switch {
	case err != nil:
		return fmt.Errorf("failed to parse file as YAML: %w", err)
	case len(file.Docs) != 1:
		return fmt.Errorf("exactly one document was expected in the YAML file, but %d were found", len(file.Docs))
	case file.Docs[0].Body == nil:
		return errors.New("the root node is nil")
	}
	root, err := as[*ast.MappingNode](file.Docs[0].Body)
	if err != nil {
		return err
	}
	linters, err := lookup[*ast.MappingNode](root, "linters")
	if err != nil {
		return err
	}
	disable, err := lookup[*ast.SequenceNode](linters, "disable")
	if err != nil {
		return err
	}
	disableValues := make([]*ast.StringNode, 0, len(disable.Values))
	for _, node := range disable.Values {
		stringNode, err := as[*ast.StringNode](node)
		if err != nil {
			return err
		}
		disableValues = append(disableValues, stringNode)
	}
	slices.SortStableFunc(disableValues, func(a, b *ast.StringNode) int {
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
	for _, value := range disableValues {
		disable.Values = append(disable.Values, value)
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

func lookup[T ast.Node](node *ast.MappingNode, key string) (T, error) {
	var found ast.Node
	for _, keyValue := range node.Values {
		if key == keyValue.Key.String() {
			found = keyValue.Value
			break
		}
	}
	if found == nil {
		return zero[T](), fmt.Errorf("the mapping node at %q does not have the key %q", node.GetPath(), key)
	}
	return as[T](found)
}

func as[T ast.Node](node ast.Node) (T, error) {
	out, ok := node.(T)
	if !ok {
		return zero[T](), fmt.Errorf("the node at %q is %s, not %s", node.GetPath(), node.Type(), out.Type())
	}
	return out, nil
}

func zero[T any]() T {
	var zero T
	return zero
}
