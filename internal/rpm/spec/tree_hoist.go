// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package spec

import (
	"regexp"
)

// hoistReferencedMacros moves [macroDefBlock] children of soon-to-be-removed
// sections to the root level when those macros are referenced by content that
// will survive removal.
//
// Motivation (issue #203): spec authors sometimes place `%define` inside a
// `%package` subpackage block (e.g. a `%define testsdir` under `%package tests`)
// even though the macro is referenced by an unconditional section like
// `%install`. Naively removing the subpackage drops the macro and leaves
// dangling `%{testsdir}` references in survivors. Hoisting preserves the
// definition just before the first removed block so the survivors still
// resolve.
//
// Hoisted blocks are inserted at the root in declaration order, immediately
// before the topmost ancestor (a direct child of root) of the first removed
// section. Macros that are NOT referenced by any survivor are left alone --
// they are subsequently dropped along with their enclosing section by the
// normal removal pass.
//
// This function mutates root in place. It must be called BEFORE removed
// blocks are detached from the tree so that the "referenced outside the
// removed subtrees" check can compute the survivor set correctly.
func hoistReferencedMacros(root *block, removed []*block) {
	if len(removed) == 0 {
		return
	}

	removedSet := blockSet(removed)

	macros := collectMacrosInSections(removed)
	if len(macros) == 0 {
		return
	}

	hoist := make([]*block, 0, len(macros))

	for _, macro := range macros {
		if isMacroReferencedOutside(root, macro.Name, removedSet) {
			hoist = append(hoist, macro)
		}
	}

	if len(hoist) == 0 {
		return
	}

	insertIdx := firstRemovedRootChildIdx(root, removedSet)
	if insertIdx < 0 {
		// Defensive: no top-level removal found (shouldn't happen if removal
		// validation passed). Prepend at root to keep the macros alive.
		insertIdx = 0
	}

	spliced := make([]*block, 0, len(root.Children)+len(hoist))
	spliced = append(spliced, root.Children[:insertIdx]...)
	spliced = append(spliced, hoist...)
	spliced = append(spliced, root.Children[insertIdx:]...)
	root.Children = spliced
}

// blockSet builds an identity-set of block pointers for O(1) lookup.
func blockSet(blocks []*block) map[*block]bool {
	set := make(map[*block]bool, len(blocks))
	for _, b := range blocks {
		set[b] = true
	}

	return set
}

// collectMacrosInSections gathers every [macroDefBlock] reachable from any of
// the given section blocks, preserving declaration order across sections.
func collectMacrosInSections(sections []*block) []*block {
	var macros []*block

	for _, sec := range sections {
		collectMacrosInBlock(sec, &macros)
	}

	return macros
}

func collectMacrosInBlock(blk *block, out *[]*block) {
	switch blk.Kind {
	case macroDefBlock:
		*out = append(*out, blk)
	case rootBlock, sectionBlock:
		for _, child := range blk.Children {
			collectMacrosInBlock(child, out)
		}
	case conditionalBlock:
		for _, child := range blk.Children {
			collectMacrosInBlock(child, out)
		}

		for _, child := range blk.Else {
			collectMacrosInBlock(child, out)
		}
	case textBlock:
		// No nested macros.
	}
}

// isMacroReferencedOutside walks the tree looking for references to name in
// any block whose enclosing section is NOT in removedSet. References include
// the standard RPM forms: %{name}, %{?name}, %{!?name}, %{name:...}, and bare
// %name terminated by a non-word character.
func isMacroReferencedOutside(root *block, name string, removedSet map[*block]bool) bool {
	pattern := macroReferencePattern(name)

	return scanForMacroReference(root, pattern, removedSet)
}

func scanForMacroReference(blk *block, pattern *regexp.Regexp, removedSet map[*block]bool) bool {
	// Skip entire subtrees rooted at a removed section.
	if blk.Kind == sectionBlock && removedSet[blk] {
		return false
	}

	switch blk.Kind {
	case textBlock, macroDefBlock:
		// A macro definition that lives outside the removed set may itself
		// reference the hoisted macro (e.g. `%define foo %{name}-suffix`).
		return anyLineMatches(blk.Lines, pattern)
	case rootBlock, sectionBlock:
		return anyChildMatches(blk.Children, pattern, removedSet)
	case conditionalBlock:
		return conditionalReferencesMacro(blk, pattern, removedSet)
	}

	return false
}

// anyLineMatches reports whether any line in lines matches pattern.
func anyLineMatches(lines []string, pattern *regexp.Regexp) bool {
	for _, line := range lines {
		if pattern.MatchString(line) {
			return true
		}
	}

	return false
}

// anyChildMatches reports whether any child block references the macro.
func anyChildMatches(children []*block, pattern *regexp.Regexp, removedSet map[*block]bool) bool {
	for _, child := range children {
		if scanForMacroReference(child, pattern, removedSet) {
			return true
		}
	}

	return false
}

// conditionalReferencesMacro reports whether the conditional itself (via its
// header or %else directive) or any of its branches reference the macro.
func conditionalReferencesMacro(cond *block, pattern *regexp.Regexp, removedSet map[*block]bool) bool {
	// The %if header itself can reference macros (e.g. `%if 0%{?with_foo}`).
	if pattern.MatchString(cond.Header) {
		return true
	}

	if cond.ElseDirective != "" && pattern.MatchString(cond.ElseDirective) {
		return true
	}

	return anyChildMatches(cond.Children, pattern, removedSet) ||
		anyChildMatches(cond.Else, pattern, removedSet)
}

// macroReferencePattern builds a regexp that matches references to a named
// RPM macro. Supported forms:
//   - %{name}, %{?name}, %{!?name}
//   - %{name:default} (parameterized expansion)
//   - bare %name terminated by a non-word character or end of string
//
// The bare form requires a word boundary so we don't match %nameOther.
func macroReferencePattern(name string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(name)
	// Braced: %{ optional ! optional ? NAME ( } | : ... )
	// Bare:   %NAME terminated by \b
	pattern := `%(?:\{!?\??` + quoted + `[}:]|` + quoted + `\b)`

	return regexp.MustCompile(pattern)
}

// firstRemovedRootChildIdx returns the index of the first child of root that
// either IS a removed block or contains a removed section in any descendant
// (including inside conditional branches). Returns -1 if no root child
// contains a removed block.
func firstRemovedRootChildIdx(root *block, removedSet map[*block]bool) int {
	for i, child := range root.Children {
		if containsRemoved(child, removedSet) {
			return i
		}
	}

	return -1
}

func containsRemoved(blk *block, removedSet map[*block]bool) bool {
	if removedSet[blk] {
		return true
	}

	switch blk.Kind {
	case rootBlock, sectionBlock:
		for _, child := range blk.Children {
			if containsRemoved(child, removedSet) {
				return true
			}
		}
	case conditionalBlock:
		for _, child := range blk.Children {
			if containsRemoved(child, removedSet) {
				return true
			}
		}

		for _, child := range blk.Else {
			if containsRemoved(child, removedSet) {
				return true
			}
		}
	case textBlock, macroDefBlock:
		// Leaves can't contain sections.
	}

	return false
}
