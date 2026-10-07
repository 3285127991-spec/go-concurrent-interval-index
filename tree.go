package intervalindex

// node is one AVL tree node. The tree is ordered by (start, id).
// Each node caches the maximum end of its subtree (maxEnd) so that
// point/overlap queries can prune whole subtrees.
type node struct {
	id     string
	start  int64
	end    int64
	value  string
	height int
	maxEnd int64
	left   *node
	right  *node
}

func lessKey(aStart int64, aID string, bStart int64, bID string) bool {
	if aStart != bStart {
		return aStart < bStart
	}
	return aID < bID
}

func heightOf(n *node) int {
	if n == nil {
		return 0
	}
	return n.height
}

func maxEndOf(n *node) int64 {
	if n == nil {
		return 0
	}
	return n.maxEnd
}

// recalc restores the cached height and maxEnd from the children.
// It must be called after any structural change below n.
func (n *node) recalc() {
	n.height = 1 + max(heightOf(n.left), heightOf(n.right))
	n.maxEnd = max(n.end, max(maxEndOf(n.left), maxEndOf(n.right)))
}

func rotateLeft(n *node) *node {
	r := n.right
	n.right = r.left
	r.left = n
	n.recalc()
	r.recalc()
	return r
}

func rotateRight(n *node) *node {
	l := n.left
	n.left = l.right
	l.right = n
	n.recalc()
	l.recalc()
	return l
}

// rebalance fixes AVL balance at n after recalculating its caches.
func rebalance(n *node) *node {
	n.recalc()
	bf := heightOf(n.left) - heightOf(n.right)
	if bf > 1 {
		if heightOf(n.left.left) < heightOf(n.left.right) {
			n.left = rotateLeft(n.left)
		}
		return rotateRight(n)
	}
	if bf < -1 {
		if heightOf(n.right.right) < heightOf(n.right.left) {
			n.right = rotateRight(n.right)
		}
		return rotateLeft(n)
	}
	return n
}

func insertNode(n *node, id string, start, end int64, value string) *node {
	if n == nil {
		return &node{id: id, start: start, end: end, value: value, height: 1, maxEnd: end}
	}
	if lessKey(start, id, n.start, n.id) {
		n.left = insertNode(n.left, id, start, end, value)
	} else {
		n.right = insertNode(n.right, id, start, end, value)
	}
	return rebalance(n)
}

func deleteNode(n *node, start int64, id string) *node {
	if n == nil {
		return nil
	}
	switch {
	case lessKey(start, id, n.start, n.id):
		n.left = deleteNode(n.left, start, id)
	case lessKey(n.start, n.id, start, id):
		n.right = deleteNode(n.right, start, id)
	default:
		if n.left == nil {
			return n.right
		}
		if n.right == nil {
			return n.left
		}
		succ := n.right
		for succ.left != nil {
			succ = succ.left
		}
		n.id, n.start, n.end, n.value = succ.id, succ.start, succ.end, succ.value
		n.right = deleteNode(n.right, succ.start, succ.id)
	}
	return rebalance(n)
}

// findPoint collects every entry whose interval contains point,
// i.e. start <= point < end, in ascending (start, id) order.
func findPoint(n *node, point int64, out *[]Entry) {
	if n == nil {
		return
	}
	// Left subtree can match only if some interval there ends past point.
	if maxEndOf(n.left) > point {
		findPoint(n.left, point, out)
	}
	if n.start <= point && point < n.end {
		*out = append(*out, Entry{ID: n.id, Start: n.start, End: n.end, Value: n.value})
	}
	// Right subtree starts at >= n.start; prune once n.start is past point.
	if n.start <= point {
		findPoint(n.right, point, out)
	}
}

// overlap collects every entry whose interval intersects [start, end)
// (half-open on both sides), in ascending (start, id) order.
func overlap(n *node, start, end int64, out *[]Entry) {
	if n == nil {
		return
	}
	// Left subtree can match only if some interval there ends after start.
	if maxEndOf(n.left) > start {
		overlap(n.left, start, end, out)
	}
	if n.start < end && start < n.end {
		*out = append(*out, Entry{ID: n.id, Start: n.start, End: n.end, Value: n.value})
	}
	// Right subtree starts at >= n.start; prune once n.start reaches end.
	if n.start < end {
		overlap(n.right, start, end, out)
	}
}

// inOrder appends all entries in ascending (start, id) order.
func inOrder(n *node, out *[]Entry) {
	if n == nil {
		return
	}
	inOrder(n.left, out)
	*out = append(*out, Entry{ID: n.id, Start: n.start, End: n.end, Value: n.value})
	inOrder(n.right, out)
}
