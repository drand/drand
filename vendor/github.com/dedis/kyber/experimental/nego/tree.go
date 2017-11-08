// +build experimental

// XXX currently this is dead code;
// keeping for now only in case I find another use for this tree impl
// in the near future.

package nego

import (
	"fmt"
	"strings"
)

type dumpable interface {
	String() string
}

// A node in a layout tree.
type node struct {
	obj      interface{} // Ciphersuite (DH) or Entry in this extent
	lo, hi   int         // Byte position range
	weight   uint32      // Pseudorandom weight for balancing
	l, r     *node       // Left,right children in tree
	conflict bool        // Set when ciphersuites conflict on position
}

// A layout tree represents a set of non-overlapping extents allocated so far
// in a negotiation header, sorted by position.
type treeLayout struct {
	root *node // Root of extent tree
}

// Count the number of 1 bits in an int:
// the metric we use to keep the layout tree moderately balanced.
func bitWeight(v int) int {
	weight := 0
	for ; v != 0; v &= v - 1 { // Clear least-significant 1-bit
		weight++
	}
	return weight
}

func (n *node) init(obj interface{}, lo, hi int, weight uint32) {
	n.obj = obj
	n.lo = lo
	n.hi = hi
	n.weight = weight
}

// Find and return any layout node overlapping this extent,
// or nil if there is no overlapping extent already in the tree.
func (n *node) find(t *node) *node {
	if n.hi <= t.lo { // n is completely below t - go left
		if t.l != nil {
			return n.find(t.l)
		}
		return nil
	} else if n.lo >= t.hi { // n is completely above t - go right
		if t.r != nil {
			return n.find(t.r)
		}
		return nil
	} else {
		return n // n overlaps t
	}
}

// Prune and return any subtree of t to the left of n.
func (n *node) pruneLeft(tp **node) *node {
	t := *tp
	if t == nil {
		return nil // no such subtree
	}
	if n.hi <= t.lo { // n is completely below t: search left
		return n.pruneLeft(&t.l)
	} else if n.lo >= t.hi { // n is completely above t: prune t
		*tp = n.pruneRight(&t.r)
		return t
	} else { // n overlaps t
		panic("prune: encountered overlapping extent")
	}
}

// Prune and return any subtree of t to the right of n.
func (n *node) pruneRight(tp **node) *node {
	t := *tp
	if t == nil {
		return nil // no such subtree
	}
	if n.hi <= t.lo { // n is completely below t: prune t
		*tp = n.pruneLeft(&t.l)
		return t
	} else if n.lo >= t.hi { // n is completely above t: search right
		return n.pruneRight(&t.r)
	} else { // n overlaps t
		panic("prune: encountered overlapping extent")
	}
}

// Insert new node to occupy the specified extent,
// if that extent does not occupy an already-allocated extent.
// Returns nil on success, or a conflicting node if position is occupied.
func (n *node) insert(tp **node) *node {
	t := *tp
	if t == nil { // trivial if root is nil
		*tp = n
		return nil
	}

	// If we've descended to a node heavier than us, insert here.
	if n.weight < t.weight {
		if c := n.find(t); c != nil {
			return c // overlapping node exists
		}
		*tp = n // insert ourselves here
		n.l = t
		n.r = n.pruneRight(&n.l)
		return nil
	}

	// Keep descending in the tree
	if n.hi <= t.lo { // n is completely below t
		return n.insert(&t.l)
	} else if n.lo >= t.hi { // n is completely above t
		return n.insert(&t.r)
	} else {
		return t // n overlaps t
	}
}

// Merge a left and a right subtree into a single subtree.
// Assumes subtree l is completely to the left of subtree r.
func join(l, r *node) *node {
	if l == nil {
		return r
	}
	if r == nil {
		return l
	}
	//	fmt.Printf("join [%d-%d] %s and [%d-%d] %s\n",
	//			l.lo, l.hi, l.obj.String(),
	//			r.lo, r.hi, r.obj.String())
	if l.weight > r.weight { // push l down into subtree r
		r.l = join(l, r.l)
		return r
	} else { // push r down into subtree l
		l.r = join(l.r, r)
		return l
	}
}

// Remove node n from tree.
func (n *node) remove(tp **node) {
	t := *tp
	if t == n { // found ourselves
		*tp = join(t.l, t.r)
		return
	}
	if t == nil {
		panic("failed to find layout node to remove")
	}
	//	fmt.Printf("remove [%d-%d] %s from [%d-%d] %s\n",
	//			n.lo, n.hi, n.obj.String(),
	//			t.lo, t.hi, t.obj.String())
	if n.hi <= t.lo { // n is completely below t
		n.remove(&t.l)
	} else if n.lo >= t.hi { // n is completely above t
		n.remove(&t.r)
	} else {
		panic("remove: found overlapping but non-equal node")
	}
}

func (n *node) scan(f func(*node)) {
	if n == nil {
		return
	}
	n.l.scan(f)
	f(n)
	n.r.scan(f)
}

// Dump the extent tree and check invariants, for debugging.
func (n *node) dump(indent int) {
	if n.lo < 0 || n.hi <= n.lo {
		panic("bad extent in layout tree")
	}
	if n.l != nil {
		n.l.dump(indent + 1)
		if n.l.hi > n.lo {
			panic("layout tree invariant failed")
		}
	}
	conf := ""
	if n.conflict {
		conf = " (CONFLICT)"
	}
	fmt.Printf("%sw%d [%d-%d] %s%s\n",
		strings.Repeat(" ", indent), n.weight,
		n.lo, n.hi, n.obj.(dumpable).String(), conf)
	if n.r != nil {
		if n.hi > n.r.lo {
			panic("layout tree invariant failed")
		}
		n.r.dump(indent + 1)
	}
}

// Initialize or clear the layout to empty.
func (l *treeLayout) init() {
	l.root = nil
}

// Find and return any node overlapping this extent in this layout,
// or nil if there is no overlapping extent in the layout.
func (l *treeLayout) find(n *node) *node {
	return n.find(l.root)
}

// Find the rightmost current node in the layout.
// Assumes the layout is non-empty.
func (l *treeLayout) top() *node {
	var n *node
	for n = l.root; n.r != nil; n = n.r {
	}
	return n
}

// Remove a node from the layout.
func (l *treeLayout) remove(n *node) {
	n.remove(&l.root)
}

// Insert new node to occupy the specified extent in this layout,
// if that extent does not occupy an already-allocated extent.
// Returns true on success, false if the position is occupied.
func (l *treeLayout) insert(n *node) *node {
	return n.insert(&l.root)
}

// Reserve an extent for a given (opaque) object.
func (l *treeLayout) reserve(obj interface{}, lo, hi int) bool {
	n := node{}
	n.init(obj, lo, hi, randUint32())
	return l.insert(&n) == nil
}

// Traverse the layout in order of increasing position.
// The supplied function f must not modify the layout.
func (l *treeLayout) scan(f func(*node)) {
	l.root.scan(f)
}

// Count the total bytes in all extents.
func (l *treeLayout) used() int {
	bytes := 0
	l.scan(func(n *node) {
		bytes += n.hi - n.lo
	})
	return bytes
}

func (l *treeLayout) dump() {
	l.root.dump(0)

	used := l.used()
	size := l.top().hi
	fmt.Printf("%d of %d bytes used, efficiency %.0f%%\n", used, size,
		float32(used*100)/float32(size))
}
