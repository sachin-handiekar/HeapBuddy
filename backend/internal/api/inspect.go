package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

const (
	directionIncoming = "incoming"
	directionOutgoing = "outgoing"

	// maxRefNodes caps how many neighbours one inspector panel (or lazy
	// expansion) returns, keeping payloads bounded for highly-referenced objects.
	maxRefNodes = 200
)

// BuildInspectorData assembles the Object Inspector view for a class or a
// specific instance. When hash is set it identifies the object (0x… hex id);
// otherwise the class's largest instance is used. Returns false if neither
// resolves to a known object.
func BuildInspectorData(g *analysis.ReferenceGraph, className, hash string) (InspectorData, bool) {
	if g == nil {
		return InspectorData{}, false
	}

	var objID uint64
	if hash != "" {
		id, ok := parseHexID(hash)
		if !ok {
			return InspectorData{}, false
		}
		objID = id
	} else {
		id, ok := g.RepresentativeInstance(className)
		if !ok {
			return InspectorData{}, false
		}
		objID = id
	}

	classID, size, ok := g.Node(objID)
	if !ok {
		return InspectorData{}, false
	}

	return InspectorData{
		ClassName:     g.ClassName(objID),
		IdentityHash:  hexID(objID),
		ShallowBytes:  size,
		RetainedBytes: size, // no dominator-based retained size yet
		Instances:     g.InstanceCount(classID),
		Fields:        buildInspectorFields(g, objID),
		Statics:       []InspectorField{}, // parser does not retain static field data
		Incoming:      refNodes(g, g.Incoming(objID), directionIncoming),
		Outgoing:      refNodes(g, g.Outgoing(objID), directionOutgoing),
	}, true
}

// BuildRefChildren returns the incoming or outgoing neighbours of the object
// identified by parentID (a 0x… hex id), for lazy tree expansion. Returns false
// for an unknown id or an invalid direction.
func BuildRefChildren(g *analysis.ReferenceGraph, parentID, direction string) ([]InspectorRefNode, bool) {
	if g == nil {
		return nil, false
	}
	objID, ok := parseHexID(parentID)
	if !ok {
		return nil, false
	}
	// An unknown-but-valid id simply has no navigable references (e.g. a target
	// that has no instance dump), so it yields an empty list rather than a 404.
	switch direction {
	case directionIncoming:
		return refNodes(g, g.Incoming(objID), directionIncoming), true
	case directionOutgoing:
		return refNodes(g, g.Outgoing(objID), directionOutgoing), true
	default:
		return nil, false
	}
}

// buildInspectorFields lists the object's object-typed instance fields (the only
// field data the parser retains). Array index "fields" are omitted here — they
// appear in the outgoing references panel instead.
func buildInspectorFields(g *analysis.ReferenceGraph, objID uint64) []InspectorField {
	out := []InspectorField{}
	for _, e := range g.Outgoing(objID) {
		if strings.HasPrefix(e.RefType, "[") {
			continue
		}
		className := g.ClassName(e.TargetId)
		field := InspectorField{Name: e.RefType, DeclaredType: className}
		if _, size, ok := g.Node(e.TargetId); ok {
			field.Target = &InspectorTargetRef{
				ClassName:     className,
				IdentityHash:  hexID(e.TargetId),
				ShallowBytes:  size,
				RetainedBytes: size,
			}
		}
		out = append(out, field)
		if len(out) >= maxRefNodes {
			break
		}
	}
	return out
}

// refNodes maps reference edges to inspector tree nodes. For incoming edges the
// node is the holder (SourceId); for outgoing edges it is the referenced object
// (TargetId). childCount is that node's own neighbour count in the same
// direction, so the UI knows whether it can expand further.
func refNodes(g *analysis.ReferenceGraph, edges []types.Reference, direction string) []InspectorRefNode {
	out := make([]InspectorRefNode, 0, len(edges))
	for _, e := range edges {
		nodeID := e.TargetId
		if direction == directionIncoming {
			nodeID = e.SourceId
		}

		_, size, _ := g.Node(nodeID)

		var childCount int
		var isRoot bool
		if direction == directionIncoming {
			childCount = len(g.Incoming(nodeID))
			isRoot = g.IsRoot(nodeID) // a GC root keeps this object alive
		} else {
			childCount = len(g.Outgoing(nodeID))
		}

		out = append(out, InspectorRefNode{
			ID:            hexID(nodeID),
			Label:         edgeLabel(e.RefType),
			ClassName:     g.ClassName(nodeID),
			IdentityHash:  hexID(nodeID),
			ShallowBytes:  size,
			RetainedBytes: size,
			ChildCount:    childCount,
			IsRoot:        isRoot,
		})
		if len(out) >= maxRefNodes {
			break
		}
	}
	return out
}

// edgeLabel renders a reference edge: array indices as "[i]", instance fields
// with a leading dot.
func edgeLabel(refType string) string {
	if refType == "" {
		return ""
	}
	if strings.HasPrefix(refType, "[") {
		return refType
	}
	return "." + refType
}

func hexID(id uint64) string { return fmt.Sprintf("0x%x", id) }

func parseHexID(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if s == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
