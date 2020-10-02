package geometry2D

import (
	"fmt"
	"testing"

	"github.com/notargets/gocfd/utils"

	"github.com/stretchr/testify/assert"

	"github.com/notargets/avs/chart2d"
	graphics2D "github.com/notargets/avs/geometry"
	utils2 "github.com/notargets/avs/utils"
)

func TestTriangulate(t *testing.T) {
	R7 := []float64{
		-0.9576571544, 0.9153143089, -0.9576571544, -0.7988312052, 0.5976624104, -0.7988312052, -0.4579233846, -0.0841532308, -0.4579233846, -0.1196174832, -0.7607650336, -0.1196174832, 0.7599592829, -0.9634866419, -0.7964726410, -0.7964726410, -0.9634866419, 0.7599592829, 0.1651240457, -0.9531905891, -0.2119334567, -0.2119334567, -0.9531905891, 0.1651240457, 0.5030612291, -0.9555522909, -0.5475089382, -0.5475089382, -0.9555522909, 0.5030612291, -0.5018415448, -0.7696328218, 0.2714743665, 0.2714743665, -0.7696328218, -0.5018415448,
		//-0.9576571544, 0.9153143089, -0.9576571544, 0.5976624104, -0.7988312052, -0.1196174832, 0.7599592829, -0.7964726410, 0.5030612291, -0.9555522909, -0.5475089382, -0.5475089382, -0.9555522909, 0.5030612291, -0.5018415448, -0.7696328218, 0.2714743665, 0.2714743665, -0.7696328218, -0.5018415448,
	}
	S7 := []float64{
		-0.9576571544, -0.9576571544, 0.9153143089, -0.7988312052, -0.7988312052, 0.5976624104, -0.4579233846, -0.4579233846, -0.0841532308, -0.7607650336, -0.1196174832, -0.1196174832, -0.9634866419, 0.7599592829, 0.7599592829, -0.9634866419, -0.7964726410, -0.7964726410, -0.9531905891, 0.1651240457, 0.1651240457, -0.9531905891, -0.2119334567, -0.2119334567, -0.9555522909, 0.5030612291, 0.5030612291, -0.9555522909, -0.5475089382, -0.5475089382, -0.7696328218, -0.5018415448, -0.5018415448, -0.7696328218, 0.2714743665, 0.2714743665,
		//-0.9576571544, -0.9576571544, 0.9153143089, -0.7988312052, 0.5976624104, -0.1196174832, -0.9634866419, -0.9634866419, -0.9555522909, 0.5030612291, 0.5030612291, -0.9555522909, -0.5475089382, -0.5475089382, -0.7696328218, -0.5018415448, -0.5018415448, -0.7696328218, 0.2714743665, 0.2714743665,
	}
	if true { //Test Add Point
		R := []float64{-1, 1, -1} // Vertices
		S := []float64{-1, -1, 1}
		tm := NewTriMesh(R, S)

		tri := &Tri{}
		tri.AddEdge(tm.NewEdge([2]int{0, 1}, true))
		e2 := tm.NewEdge([2]int{1, 2}, true)
		tri.AddEdge(e2)
		tri.AddEdge(tm.NewEdge([2]int{2, 0}, true))
		tm.AddBoundingTriangle(tri)
		verts, _ := tri.GetVertices()
		assert.Equal(t, verts, [3]int{0, 1, 2})

		if true {
			RR := R7
			SS := S7
			for i := range RR {
				tm.AddPoint(RR[i], SS[i])
			}
		} else {
			/*
				tm.AddPoint(-0.33, -0.33)
				tm.AddPoint(-.15, -.15)
				tm.AddPoint(0.75, -0.75)
			*/
			tm.AddPoint(-0.33, -0.33)
			tm.AddPoint(-.25, -.75)
			tm.AddPoint(-.15, -.15)
			tm.AddPoint(-.75, -.25)
			tm.AddPoint(-1.0, 0.0)
			tm.AddPoint(-1.0, -0.5)
			tm.AddPoint(-0.5, -1)
			tm.AddPoint(0.75, -0.75)
			tm.AddPoint(0.25, -0.25)
			tm.AddPoint(0.5, -0.5)
			tm.AddPoint(0.0, 0.0)
			tm.AddPoint(-0.25, 0.25)
			tm.AddPoint(-0.5, 0.5)
			tm.AddPoint(-0.75, 0.75)
			tm.AddPoint(0.0, -1)
			tm.AddPoint(0.5, -1)
			tm.AddPoint(-1, 0.5)
		}
		/*
			tm.TriGraph.PrintDotOutput()
			fmt.Printf("Points============================\n")
			for i, pt := range tm.Points {
				fmt.Printf("%d:%s\n", i, pt.Print())
			}
		*/
		//gm := tm.ToGraphMesh()
		//fmt.Println("number of finished tris = ", len(gm.Triangles))
		var withTris, withTrisAndImmovable int
		for _, ee := range tm.Edges {
			if len(ee.Tris) != 0 {
				//fmt.Printf("With %d Tris: ", len(ee.Tris))
				withTris++
				if ee.IsImmovable {
					withTrisAndImmovable++
				}
			}
			//fmt.Printf("Edge[%d] = %s\n", i, ee.Print())
		}
		fmt.Printf("Number of edges with tris: %d, Immovable edges with tris: %d\n",
			withTris, withTrisAndImmovable)
		plot := false
		if plot {
			if false {
				gm := tm.ToGraphMesh()
				for i, tri := range gm.Triangles {
					fmt.Printf("tri[%d] = %v\n", i, tri)
					for _, atts := range gm.Attributes[i] {
						fmt.Printf("atts[%d] = %v\n", i, atts)
					}
				}
			}
			plotTriangles(tm.ToGraphMesh())
			utils.SleepFor(100000)
		}
	}
	if false { //Test Legal Edge test
		R := []float64{-0.9600, 0.9201, -0.9600, -0.7366, 0.4731, -0.7366, -0.3333, -0.0297, -0.9405, -0.0297, 0.7358, -0.9517, -0.7841, -0.7841, -0.9517, 0.7358, 0.4017, -0.9434, -0.4583, -0.4583, -0.9434, 0.4017, 0.0733, -0.7064, -0.3669, -0.3669, -0.7064, 0.0733, -0.9600, 0.9201, -0.9600, -0.7366, 0.4731, -0.7366, -0.3333, -0.0297, -0.9405, -0.0297, 0.7358, -0.9517, -0.7841, -0.7841, -0.9517, 0.7358, 0.4017, -0.9434, -0.4583, -0.4583, -0.9434, 0.4017, 0.0733, -0.7064, -0.3669, -0.3669, -0.7064, 0.0733, 0.9195, 0.7388, 0.4779, 0.1653, -0.1653, -0.4779, -0.7388, -0.9195, -1.0000, -1.0000, -1.0000, -1.0000, -1.0000, -1.0000, -1.0000, -1.0000, -0.9195, -0.7388, -0.4779, -0.1653, 0.1653, 0.4779, 0.7388, 0.9195}
		S := []float64{-0.9600, -0.9600, 0.9201, -0.7366, -0.7366, 0.4731, -0.3333, -0.9405, -0.0297, -0.0297, -0.9517, 0.7358, 0.7358, -0.9517, -0.7841, -0.7841, -0.9434, 0.4017, 0.4017, -0.9434, -0.4583, -0.4583, -0.7064, 0.0733, 0.0733, -0.7064, -0.3669, -0.3669, -0.9600, -0.9600, 0.9201, -0.7366, -0.7366, 0.4731, -0.3333, -0.9405, -0.0297, -0.0297, -0.9517, 0.7358, 0.7358, -0.9517, -0.7841, -0.7841, -0.9434, 0.4017, 0.4017, -0.9434, -0.4583, -0.4583, -0.7064, 0.0733, 0.0733, -0.7064, -0.3669, -0.3669, -0.9195, -0.7388, -0.4779, -0.1653, 0.1653, 0.4779, 0.7388, 0.9195, -0.9195, -0.7388, -0.4779, -0.1653, 0.1653, 0.4779, 0.7388, 0.9195, -1.0000, -1.0000, -1.0000, -1.0000, -1.0000, -1.0000, -1.0000, -1.0000}
		R = append(R, -1, 1, -1) // Vertices
		S = append(S, -1, -1, 1)
		R = append(R, -0.9999999) // Almost at vertex, but inside
		S = append(S, -1)
		testCheck := make([]bool, len(R))
		for i := range testCheck {
			testCheck[i] = true
		}
		testCheck[len(R)-4] = false // vertices will be legal
		testCheck[len(R)-3] = false
		testCheck[len(R)-2] = false
		// Check expected results, most points are inside the circumscribing circle, so will be "illegal"
		for i, r := range R {
			s := S[i]
			fmt.Printf("point[%8.5f,%8.5f] ", r, s)
			assert.Equal(t, IsIllegalEdge(r, s, -1, -1, 1, -1, -1, 1), testCheck[i])
			if testCheck[i] {
				// testCheck is true, so edge is illegal, inside
				fmt.Printf("is inside, illegal (ccw), ")
			} else {
				fmt.Printf("is outside, legal (ccw), ")
			}
			// Test the opposite direction of the base triangle
			assert.Equal(t, IsIllegalEdge(r, s, -1, 1, 1, -1, -1, -1), testCheck[i])
			if testCheck[i] {
				// testCheck is true, so edge is illegal, inside
				fmt.Printf("is inside, illegal\n")
			} else {
				fmt.Printf("is outside, legal\n")
			}
		}
	}
}

func plotTriangles(triMesh graphics2D.TriMesh) (chart *chart2d.Chart2D) {
	colorMap := utils2.NewColorMap(0, 1, 1)
	box := graphics2D.NewBoundingBox(triMesh.GetGeometry())
	chart = chart2d.NewChart2D(1024, 1024, box.XMin[0], box.XMax[0], box.XMin[1], box.XMax[1])
	chart.AddColorMap(colorMap)
	go chart.Plot()

	updateTriMesh(chart, triMesh)
	return
}

func updateTriMesh(chart *chart2d.Chart2D, triMesh graphics2D.TriMesh) {
	if err := chart.AddTriMesh("TriMesh", triMesh.Geometry, triMesh,
		chart2d.CrossGlyph, chart2d.Solid, utils.GetColor(utils.White)); err != nil {
		panic("unable to add graph series")
	}
}
