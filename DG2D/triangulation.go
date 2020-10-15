package DG2D

import (
	"fmt"
	"math"

	"github.com/notargets/gocfd/utils"
)

type Triangulation struct {
	EToV  utils.Matrix         // K x 3 matrix mapping vertices to triangles
	Edges map[EdgeNumber]*Edge // map of edges, key is the edge number, an int packed with the two vertices of each edge
}

func NewTriangulation(EToV utils.Matrix) (tmesh *Triangulation) {
	tmesh = &Triangulation{
		EToV:  EToV,
		Edges: make(map[EdgeNumber]*Edge),
	}
	K, _ := EToV.Dims()
	for k := 0; k < K; k++ {
		tri := EToV.Row(k).Data()
		verts := [3]int{int(tri[0]), int(tri[1]), int(tri[2])}
		// Create / store the edges for this triangle
		tmesh.NewEdge([2]int{verts[0], verts[1]}, k, 0)
		tmesh.NewEdge([2]int{verts[1], verts[2]}, k, 1)
		tmesh.NewEdge([2]int{verts[2], verts[0]}, k, 2)
	}
	return
}

func (tmesh *Triangulation) NewEdge(verts [2]int, connectedElementNumber int, intEdgeNumber InternalEdgeNumber) (e *Edge) {
	var (
		ok bool
	)
	/*
		The input vertices are ordered as the normal traversal within the triangle
	*/
	// Determine edge direction
	var dir InternalEdgeDirection
	if verts[0] > verts[1] {
		dir = Reversed
	}
	// Check if edge is already stored
	en := NewEdgeNumber(verts)
	if e, ok = tmesh.Edges[en]; !ok {
		e = &Edge{
			NumConnectedTris:       1,
			ConnectedTris:          [2]uint32{uint32(connectedElementNumber)},
			ConnectedTriDirection:  [2]InternalEdgeDirection{dir},
			ConnectedTriEdgeNumber: [2]InternalEdgeNumber{intEdgeNumber},
		}
		tmesh.Edges[en] = e
	} else {
		e.NumConnectedTris++
		e.ConnectedTris[1] = uint32(connectedElementNumber)
		e.ConnectedTriDirection[1] = dir
		e.ConnectedTriEdgeNumber[1] = intEdgeNumber
	}
	return
}

type Edge struct {
	// Storage: 16 bytes (64 bit aligned)
	NumConnectedTris       uint8                    // Either 1 or 2
	ConnectedTris          [2]uint32                // Index numbers of triangles connected to this edge
	ConnectedTriDirection  [2]InternalEdgeDirection // If false(default), the edge runs from smaller to larger within the connected tri
	ConnectedTriEdgeNumber [2]InternalEdgeNumber    // For the connected triangles, what is the edge number (one of 0, 1 or 2)
	BCType                 BCFLAG                   // If not connected to two tris, this field will be used
}

func (e *Edge) Print() (p string) {
	//for i, triNum := range e.ConnectedTris {
	for i := 0; i < int(e.NumConnectedTris); i++ {
		triNum := e.ConnectedTris[i]
		pp := fmt.Sprintf("Tri[%d] Edge[%d] Reversed?%v,",
			triNum, e.ConnectedTriEdgeNumber[i], e.ConnectedTriDirection[i])
		p += pp
	}
	return
}

type InternalEdgeNumber uint8

const (
	First InternalEdgeNumber = iota
	Second
	Third
)

type InternalEdgeDirection bool

const (
	SmallestToLargest InternalEdgeDirection = false // Edge runs smallest vertex index to largest within triangle
	Reversed          InternalEdgeDirection = true
)

type EdgeNumber uint64

func NewEdgeNumber(verts [2]int) (packed EdgeNumber) {
	// This packs two index coordinates into two 32 bit unsigned integers to act as a hash and an indirect access method
	var (
		limit = math.MaxUint32
	)
	for _, vert := range verts {
		if vert < 0 || vert > limit {
			panic(fmt.Errorf("unable to pack two ints into a uint64, have %d and %d as inputs",
				verts[0], verts[1]))
		}
	}
	var i1, i2 int
	if verts[0] < verts[1] {
		i1, i2 = verts[0], verts[1]
	} else {
		i1, i2 = verts[1], verts[0]
	}
	packed = EdgeNumber(i1 + i2<<32)
	return
}

func (en EdgeNumber) GetVertices() (verts [2]int) {
	var (
		enTmp EdgeNumber
	)
	enTmp = en >> 32
	verts[1] = int(enTmp)
	verts[0] = int(en - enTmp*(1<<32))
	return
}
