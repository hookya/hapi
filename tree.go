// Copyright 2013 Julien Schmidt. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/julienschmidt/httprouter/blob/master/LICENSE

package hapi

import (
	"github.com/hookya/hapi/internal/bytesconv"
)

type methodTree struct {
	method string
	root   *node
}

type methodTrees []methodTree

func (trees methodTrees) get(method string) *node {
	for _, tree := range trees {
		if tree.method == method {
			return tree.root
		}
	}
	return nil
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func longestCommonPrefix(a, b string) int {
	i := 0
	max := min(len(a), len(b))
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}

type node struct {
	path    string
	indices string
	// wildChild bool
	// nType    nodeType
	priority uint32
	children []*node // child nodes, at most 1 :param style node at the end of the array
	handlers HandlersChain
	fullPath string
}

// Increments priority of the given child and reorders if necessary
func (n *node) incrementChildPrio(pos int) int {
	cs := n.children
	cs[pos].priority++
	prio := cs[pos].priority

	// Adjust position (move to front)
	newPos := pos
	for ; newPos > 0 && cs[newPos-1].priority < prio; newPos-- {
		// Swap node positions
		cs[newPos-1], cs[newPos] = cs[newPos], cs[newPos-1]
	}

	// Build new index char string
	if newPos != pos {
		n.indices = n.indices[:newPos] + // Unchanged prefix, might be empty
			n.indices[pos:pos+1] + // The index char we move
			n.indices[newPos:pos] + n.indices[pos+1:] // Rest without char at 'pos'
	}

	return newPos
}

// addRoute adds a node with the given handle to the path.
// Not concurrency-safe!
func (n *node) addRoute(path string, handlers HandlersChain) {
	fullPath := path
	n.priority++

	// Empty tree
	if len(n.path) == 0 && len(n.children) == 0 {
		n.insertChild(path, fullPath, handlers)
		// n.nType = root
		return
	}

walk:
	for {
		// Find the longest common prefix.
		// This also implies that the common prefix contains no ':' or '*'
		// since the existing key can't contain those chars.
		i := longestCommonPrefix(path, n.path)

		// Split edge
		if i < len(n.path) {
			child := node{
				path: n.path[i:],
				// wildChild: n.wildChild,
				indices:  n.indices,
				children: n.children,
				handlers: n.handlers,
				priority: n.priority - 1,
				fullPath: n.fullPath,
			}

			n.children = []*node{&child}
			// []byte for proper unicode char conversion, see #65
			n.indices = bytesconv.BytesToString([]byte{n.path[i]})
			n.path = path[:i]
			n.handlers = nil
			// n.wildChild = false
		}

		// Make new node a child of this node
		if i < len(path) {
			path = path[i:]
			c := path[0]

			// '/' after param
			if c == '/' && len(n.children) == 1 {
				n = n.children[0]
				n.priority++
				continue walk
			}

			// Check if a child with the next path byte exists
			for i, max := 0, len(n.indices); i < max; i++ {
				if c == n.indices[i] {
					i = n.incrementChildPrio(i)
					n = n.children[i]
					continue walk
				}
			}

			child := &node{
				fullPath: fullPath,
			}
			n.indices += bytesconv.BytesToString([]byte{c})
			n.addChild(child)
			n.incrementChildPrio(len(n.indices) - 1)
			n = child
			n.insertChild(path, fullPath, handlers)
			return
		}

		// Otherwise add handle to current node
		if n.handlers != nil {
			panic("handlers are already registered for path '" + fullPath + "'")
		}
		n.handlers = handlers
		n.fullPath = fullPath
		return
	}
}

func (n *node) insertChild(path string, fullPath string, handlers HandlersChain) {
	// If no wildcard was found, simply insert the path and handle
	n.path = path
	n.handlers = handlers
	n.fullPath = fullPath
}

// addChild will add a child node, keeping wildcardChild at the end
func (n *node) addChild(child *node) {
	n.children = append(n.children, child)
}

// Returns the handle registered with the given path (key). The values of
// wildcards are saved to a map.
// If no handle can be found, a TSR (trailing slash redirect) recommendation is
// made if a handle exists with an extra (without the) trailing slash for the
// given path.
func (n *node) getValue(path string) (handlers HandlersChain) {

walk: // Outer loop for walking the tree
	for {
		prefix := n.path
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]

				// Try all the non-wildcard children first by matching the indices
				idxc := path[0]
				for i, c := range []byte(n.indices) {
					if c == idxc {
						n = n.children[i]
						continue walk
					}
				}
				return nil
			}
		}

		if path == prefix {
			handlers = n.handlers
			return
		}

		return
	}
}

// // Makes a case-insensitive lookup of the given path and tries to find a handler.
// // It can optionally also fix trailing slashes.
// // It returns the case-corrected path and a bool indicating whether the lookup
// // was successful.
// func (n *node) findCaseInsensitivePath(path string, fixTrailingSlash bool) ([]byte, bool) {
// 	const stackBufSize = 128

// 	// Use a static sized buffer on the stack in the common case.
// 	// If the path is too long, allocate a buffer on the heap instead.
// 	buf := make([]byte, 0, stackBufSize)
// 	if length := len(path) + 1; length > stackBufSize {
// 		buf = make([]byte, 0, length)
// 	}

// 	ciPath := n.findCaseInsensitivePathRec(
// 		path,
// 		buf,       // Preallocate enough memory for new path
// 		[4]byte{}, // Empty rune buffer
// 		fixTrailingSlash,
// 	)

// 	return ciPath, ciPath != nil
// }

// // Shift bytes in array by n bytes left
// func shiftNRuneBytes(rb [4]byte, n int) [4]byte {
// 	switch n {
// 	case 0:
// 		return rb
// 	case 1:
// 		return [4]byte{rb[1], rb[2], rb[3], 0}
// 	case 2:
// 		return [4]byte{rb[2], rb[3]}
// 	case 3:
// 		return [4]byte{rb[3]}
// 	default:
// 		return [4]byte{}
// 	}
// }

// // Recursive case-insensitive lookup function used by n.findCaseInsensitivePath
// func (n *node) findCaseInsensitivePathRec(path string, ciPath []byte, rb [4]byte, fixTrailingSlash bool) []byte {
// 	npLen := len(n.path)

// walk: // Outer loop for walking the tree
// 	for len(path) >= npLen && (npLen == 0 || strings.EqualFold(path[1:npLen], n.path[1:])) {
// 		// Add common prefix to result
// 		oldPath := path
// 		path = path[npLen:]
// 		ciPath = append(ciPath, n.path...)

// 		if len(path) == 0 {
// 			// We should have reached the node containing the handle.
// 			// Check if this node has a handle registered.
// 			if n.handlers != nil {
// 				return ciPath
// 			}

// 			// No handle found.
// 			// Try to fix the path by adding a trailing slash
// 			if fixTrailingSlash {
// 				for i, c := range []byte(n.indices) {
// 					if c == '/' {
// 						n = n.children[i]
// 						if (len(n.path) == 1 && n.handlers != nil) ||
// 							(n.nType == catchAll && n.children[0].handlers != nil) {
// 							return append(ciPath, '/')
// 						}
// 						return nil
// 					}
// 				}
// 			}
// 			return nil
// 		}

// 		// If this node does not have a wildcard (param or catchAll) child,
// 		// we can just look up the next child node and continue to walk down
// 		// the tree
// 		if !n.wildChild {
// 			// Skip rune bytes already processed
// 			rb = shiftNRuneBytes(rb, npLen)

// 			if rb[0] != 0 {
// 				// Old rune not finished
// 				idxc := rb[0]
// 				for i, c := range []byte(n.indices) {
// 					if c == idxc {
// 						// continue with child node
// 						n = n.children[i]
// 						npLen = len(n.path)
// 						continue walk
// 					}
// 				}
// 			} else {
// 				// Process a new rune
// 				var rv rune

// 				// Find rune start.
// 				// Runes are up to 4 byte long,
// 				// -4 would definitely be another rune.
// 				var off int
// 				for max := min(npLen, 3); off < max; off++ {
// 					if i := npLen - off; utf8.RuneStart(oldPath[i]) {
// 						// read rune from cached path
// 						rv, _ = utf8.DecodeRuneInString(oldPath[i:])
// 						break
// 					}
// 				}

// 				// Calculate lowercase bytes of current rune
// 				lo := unicode.ToLower(rv)
// 				utf8.EncodeRune(rb[:], lo)

// 				// Skip already processed bytes
// 				rb = shiftNRuneBytes(rb, off)

// 				idxc := rb[0]
// 				for i, c := range []byte(n.indices) {
// 					// Lowercase matches
// 					if c == idxc {
// 						// must use a recursive approach since both the
// 						// uppercase byte and the lowercase byte might exist
// 						// as an index
// 						if out := n.children[i].findCaseInsensitivePathRec(
// 							path, ciPath, rb, fixTrailingSlash,
// 						); out != nil {
// 							return out
// 						}
// 						break
// 					}
// 				}

// 				// If we found no match, the same for the uppercase rune,
// 				// if it differs
// 				if up := unicode.ToUpper(rv); up != lo {
// 					utf8.EncodeRune(rb[:], up)
// 					rb = shiftNRuneBytes(rb, off)

// 					idxc := rb[0]
// 					for i, c := range []byte(n.indices) {
// 						// Uppercase matches
// 						if c == idxc {
// 							// Continue with child node
// 							n = n.children[i]
// 							npLen = len(n.path)
// 							continue walk
// 						}
// 					}
// 				}
// 			}

// 			// Nothing found. We can recommend to redirect to the same URL
// 			// without a trailing slash if a leaf exists for that path
// 			if fixTrailingSlash && path == "/" && n.handlers != nil {
// 				return ciPath
// 			}
// 			return nil
// 		}

// 		n = n.children[0]
// 		switch n.nType {
// 		case param:
// 			// Find param end (either '/' or path end)
// 			end := 0
// 			for end < len(path) && path[end] != '/' {
// 				end++
// 			}

// 			// Add param value to case insensitive path
// 			ciPath = append(ciPath, path[:end]...)

// 			// We need to go deeper!
// 			if end < len(path) {
// 				if len(n.children) > 0 {
// 					// Continue with child node
// 					n = n.children[0]
// 					npLen = len(n.path)
// 					path = path[end:]
// 					continue
// 				}

// 				// ... but we can't
// 				if fixTrailingSlash && len(path) == end+1 {
// 					return ciPath
// 				}
// 				return nil
// 			}

// 			if n.handlers != nil {
// 				return ciPath
// 			}

// 			if fixTrailingSlash && len(n.children) == 1 {
// 				// No handle found. Check if a handle for this path + a
// 				// trailing slash exists
// 				n = n.children[0]
// 				if n.path == "/" && n.handlers != nil {
// 					return append(ciPath, '/')
// 				}
// 			}

// 			return nil

// 		case catchAll:
// 			return append(ciPath, path...)

// 		default:
// 			panic("invalid node type")
// 		}
// 	}

// 	// Nothing found.
// 	// Try to fix the path by adding / removing a trailing slash
// 	if fixTrailingSlash {
// 		if path == "/" {
// 			return ciPath
// 		}
// 		if len(path)+1 == npLen && n.path[len(path)] == '/' &&
// 			strings.EqualFold(path[1:], n.path[1:len(path)]) && n.handlers != nil {
// 			return append(ciPath, n.path...)
// 		}
// 	}
// 	return nil
// }
