package api

import (
	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
)

// maxDomNodes caps how many siblings a dominator-tree level returns, keeping
// payloads bounded; nodes are already sorted by retained size so the rest are
// smaller.
const maxDomNodes = 200

// BuildDominatorRoots returns the top-level dominator-tree nodes (objects the
// virtual root dominates), ordered by retained size.
func BuildDominatorRoots(dt *analysis.DominatorTree) []DomNode {
	if dt == nil {
		return []DomNode{}
	}
	return domNodes(dt, dt.Roots())
}

// BuildDominatorChildren returns the dominator-tree children of the object
// identified by parentID (a 0x… hex id), for lazy expansion. ok is false only
// for an unparseable id; a valid id with no children yields an empty list.
func BuildDominatorChildren(dt *analysis.DominatorTree, parentID string) ([]DomNode, bool) {
	if dt == nil {
		return nil, false
	}
	id, ok := parseHexID(parentID)
	if !ok {
		return nil, false
	}
	return domNodes(dt, dt.Children(id)), true
}

func domNodes(dt *analysis.DominatorTree, ids []uint64) []DomNode {
	out := make([]DomNode, 0, min(len(ids), maxDomNodes))
	for _, id := range ids {
		if len(out) >= maxDomNodes {
			break
		}
		shallow := dt.Shallow(id)
		out = append(out, DomNode{
			ID:            hexID(id),
			ClassName:     dt.ClassName(id),
			IdentityHash:  hexID(id),
			ShallowBytes:  shallow,
			RetainedBytes: dt.Retained(id),
			PercentOfHeap: dt.PercentOfHeap(id),
			ChildCount:    dt.ChildCount(id),
		})
	}
	return out
}
