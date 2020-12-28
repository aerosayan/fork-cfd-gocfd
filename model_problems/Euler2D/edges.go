package Euler2D

import (
	"math"
	"sort"
	"sync"

	"github.com/notargets/gocfd/DG2D"
	"github.com/notargets/gocfd/types"
	"github.com/notargets/gocfd/utils"
)

type EdgeKeySlice []types.EdgeKey

func (p EdgeKeySlice) Len() int           { return len(p) }
func (p EdgeKeySlice) Less(i, j int) bool { return p[i] < p[j] }
func (p EdgeKeySlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Sort is a convenience method.
func (p EdgeKeySlice) Sort() { sort.Sort(p) }

func (c *Euler) ParallelSetNormalFluxOnEdges(Time float64) {
	var (
		Ntot = len(c.SortedEdgeKeys)
		wg   = sync.WaitGroup{}
	)
	var ind, end int
	for n := 0; n < c.ParallelDegree; n++ {
		ind, end = c.split1D(Ntot, n)
		wg.Add(1)
		go func(ind, end int) {
			c.SetNormalFluxOnEdges(Time, c.SortedEdgeKeys[ind:end])
			wg.Done()
		}(ind, end)
	}
	wg.Wait()
}

func (c *Euler) SetNormalFluxOnEdges(Time float64, edgeKeys EdgeKeySlice) {
	var (
		Nedge                          = c.dfr.FluxElement.Nedge
		normalFlux, normalFluxReversed = make([][4]float64, Nedge), make([][4]float64, Nedge)
	)
	for _, en := range edgeKeys {
		e := c.dfr.Tris.Edges[en]
		//for en, e := range dfr.Tris.Edges {
		switch e.NumConnectedTris {
		case 0:
			panic("unable to handle unconnected edges")
		case 1: // Handle edges with only one triangle - default is edge flux, which will be replaced by a BC flux
			var (
				k                   = int(e.ConnectedTris[0])
				edgeNumber          = int(e.ConnectedTriEdgeNumber[0])
				shift               = edgeNumber * Nedge
				calculateNormalFlux bool
				qfD                 = Get4DP(c.Q_Face)
			)
			calculateNormalFlux = true
			normal, _ := c.getEdgeNormal(0, e, en)
			switch e.BCType {
			case types.BC_Far:
				c.FarBC(k, shift, normal)
			case types.BC_IVortex:
				c.IVortexBC(Time, k, shift, normal)
			case types.BC_Wall, types.BC_Cyl:
				calculateNormalFlux = false
				c.WallBC(k, shift, normal, normalFlux) // Calculates normal flux directly
			case types.BC_PeriodicReversed, types.BC_Periodic:
				// One edge of the Periodic BC leads to calculation of both sides within the connected tris section, so noop here
				calculateNormalFlux = false
			}
			if calculateNormalFlux {
				var Fx, Fy [4]float64
				for i := 0; i < Nedge; i++ {
					ie := i + shift
					Fx, Fy = c.CalculateFlux(k, ie, qfD)
					for n := 0; n < 4; n++ {
						normalFlux[i][n] = normal[0]*Fx[n] + normal[1]*Fy[n]
					}
				}
			}
			c.SetNormalFluxOnRTEdge(k, edgeNumber, normalFlux, e.IInII[0])
		case 2: // Handle edges with two connected tris - shared faces
			var (
				kL, kR                   = int(e.ConnectedTris[0]), int(e.ConnectedTris[1])
				edgeNumberL, edgeNumberR = int(e.ConnectedTriEdgeNumber[0]), int(e.ConnectedTriEdgeNumber[1])
				shiftL, shiftR           = edgeNumberL * Nedge, edgeNumberR * Nedge
			)
			switch c.FluxCalcAlgo {
			case FLUX_Average:
				normal, _ := c.getEdgeNormal(0, e, en)
				c.LaxFlux(kL, kR, shiftL, shiftR, normal, normalFlux, normalFluxReversed)
			case FLUX_LaxFriedrichs:
				normal, _ := c.getEdgeNormal(0, e, en)
				c.LaxFlux(kL, kR, shiftL, shiftR, normal, normalFlux, normalFluxReversed)
			case FLUX_Roe:
				normal, _ := c.getEdgeNormal(0, e, en)
				c.RoeFlux(kL, kR, shiftL, shiftR, normal, normalFlux, normalFluxReversed)
			}
			c.SetNormalFluxOnRTEdge(kL, edgeNumberL, normalFlux, e.IInII[0])
			c.SetNormalFluxOnRTEdge(kR, edgeNumberR, normalFluxReversed, e.IInII[1])
		}
	}
	return
}

func (c *Euler) getEdgeNormal(conn int, e *DG2D.Edge, en types.EdgeKey) (normal, scaledNormal [2]float64) {
	var (
		dfr = c.dfr
	)
	norm := func(vec [2]float64) (n float64) {
		n = math.Sqrt(vec[0]*vec[0] + vec[1]*vec[1])
		return
	}
	normalize := func(vec [2]float64) (normed [2]float64) {
		n := norm(vec)
		for i := 0; i < 2; i++ {
			normed[i] = vec[i] / n
		}
		return
	}
	revDir := bool(e.ConnectedTriDirection[conn])
	x1, x2 := DG2D.GetEdgeCoordinates(en, revDir, dfr.VX, dfr.VY)
	dx := [2]float64{x2[0] - x1[0], x2[1] - x1[1]}
	normal = normalize([2]float64{-dx[1], dx[0]})
	scaledNormal[0] = normal[0] * e.IInII[conn]
	scaledNormal[1] = normal[1] * e.IInII[conn]
	return
}

func (c *Euler) ProjectFluxToEdge(edgeFlux [][2][4]float64, e *DG2D.Edge, en types.EdgeKey) {
	/*
				Projects a 2D flux: [F, G] onto the face normal, then multiplies by the element/edge rqtio of normals, ||n||
		 		And places the scaled and projected normal flux into the degree of freedom F_RT_DOF
	*/
	var (
		dfr        = c.dfr
		Nedge      = dfr.FluxElement.Nedge
		Nint       = dfr.FluxElement.Nint
		Kmax       = dfr.K
		conn       = 0
		k          = int(e.ConnectedTris[conn])
		edgeNumber = int(e.ConnectedTriEdgeNumber[conn])
		shift      = edgeNumber * Nedge
	)
	// Get scaling factor ||n|| for each edge, multiplied by untransformed normals
	_, scaledNormal := c.getEdgeNormal(conn, e, en)
	for i := 0; i < Nedge; i++ {
		iL := i + shift
		// Project the flux onto the scaled scaledNormal
		for n := 0; n < 4; n++ {
			// Place normed/scaled flux into the RT element space
			rtD := c.F_RT_DOF[n].Data()
			ind := k + (2*Nint+iL)*Kmax
			rtD[ind] = scaledNormal[0]*edgeFlux[i][0][n] + scaledNormal[1]*edgeFlux[i][1][n]
		}
	}
}

func (c *Euler) SetNormalFluxOnRTEdge(k, edgeNumber int, edgeNormalFlux [][4]float64, IInII float64) {
	/*
		Takes the normal flux (aka "projected flux") multiplies by the ||n|| ratio of edge normals and sets that value for
		the F_RT_DOF degree of freedom locations for this [k, edgenumber] group
	*/
	var (
		dfr   = c.dfr
		Nedge = dfr.FluxElement.Nedge
		Nint  = dfr.FluxElement.Nint
		Kmax  = dfr.K
		shift = edgeNumber * Nedge
	)
	// Get scaling factor ||n|| for each edge, multiplied by untransformed normals
	for n := 0; n < 4; n++ {
		rtD := c.F_RT_DOF[n].Data()
		for i := 0; i < Nedge; i++ {
			// Place normed/scaled flux into the RT element space
			ind := k + (2*Nint+i+shift)*Kmax
			rtD[ind] = edgeNormalFlux[i][n] * IInII
		}
	}
}

func (c *Euler) InterpolateSolutionToEdges(Q [4]utils.Matrix) (Q_Face [4]utils.Matrix) {
	// Interpolate from solution points to edges using precomputed interpolation matrix
	for n := 0; n < 4; n++ {
		Q_Face[n] = c.dfr.FluxEdgeInterpMatrix.Mul(Q[n])
	}
	return
}
