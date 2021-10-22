package sod_shock_tube

import (
	"fmt"
	"sync"
	"time"

	"github.com/notargets/gocfd/DG2D"

	"github.com/notargets/gocfd/model_problems/Euler1D"

	"github.com/notargets/avs/chart2d"
	utils2 "github.com/notargets/avs/utils"
	"github.com/notargets/gocfd/utils"
)

type InterpolationTarget struct {
	ElementNumber       int          // Global K element address
	RS                  [2]float64   // Coordinates within element
	InterpolationMatrix utils.Matrix // Np x 1, Calculated from [R,S] coordinates
}

type SODShockTube struct {
	XLocations          []float64 // Locations of values for plotting
	InterpolationTarget []InterpolationTarget
	Rho, RhoU, E        []float64 // Interpolated values from solution, used for validation
	Npts                int       // Npts = # Xlocations, Np = Interior solution polynomial nodes
	DFR2D               *DG2D.DFR2D
	plotOnce            sync.Once
	chart               *chart2d.Chart2D
	colorMap            *utils2.ColorMap
}

func NewSODShockTube(nPts int, dfr *DG2D.DFR2D) (st *SODShockTube) {
	st = &SODShockTube{
		XLocations:          make([]float64, nPts),
		InterpolationTarget: make([]InterpolationTarget, nPts),
		Npts:                nPts,
		Rho:                 make([]float64, nPts),
		RhoU:                make([]float64, nPts),
		E:                   make([]float64, nPts),
		DFR2D:               dfr,
	}
	xfrac := 1. / float64(nPts-1) // Equal spaced samples across [0->1]
	for i := range st.XLocations {
		st.XLocations[i] = float64(i) * xfrac
	}
	st.calculateInterpolation()
	return
}

func (st *SODShockTube) calculateInterpolation() {
	var (
		dfr    = st.DFR2D
		getInt = dfr.SolutionElement.Simplex2DInterpolatingPolyMatrix
		VY     = dfr.VY
		// Get the centerline Y coordinate
		ymid = 0.5*(VY.Max()-VY.Min()) + VY.Min()
	)
	/*
		For each point in XLocations, find the element containing and store the coordinates
	*/
	for i, x := range st.XLocations {
		it := st.InterpolationTarget
		it[i].ElementNumber, it[i].RS[0], it[i].RS[1] = st.getUVCoords(x, ymid)
		it[i].InterpolationMatrix = getInt(
			utils.NewVector(1, []float64{it[i].RS[0]}),
			utils.NewVector(1, []float64{it[i].RS[1]}),
		)
	}
}

func (st *SODShockTube) getUVCoords(x, y float64) (ElementID int, r, s float64) {
	var (
		Kmax, _    = st.DFR2D.Tris.EToV.Dims()
		verts      [3]int
		A, B, C    [2]float64 // vertex X,Y coords
		VX, VY     = st.DFR2D.VX.DataP, st.DFR2D.VY.DataP
		v0, v1, v2 [2]float64
		P          = [2]float64{x, y}
		EToV       = st.DFR2D.Tris.EToV
	)
	minus := func(a, b [2]float64) (c [2]float64) {
		c[0] = a[0] - b[0]
		c[1] = a[1] - b[1]
		return
	}
	dot := func(a, b [2]float64) (f float64) {
		f = a[0]*b[0] + a[1]*b[1]
		return
	}
	for k := 0; k < Kmax; k++ {
		tri := EToV.Row(k).DataP
		verts = [3]int{int(tri[0]), int(tri[1]), int(tri[2])}
		A = [2]float64{VX[verts[0]], VY[verts[0]]}
		B = [2]float64{VX[verts[1]], VY[verts[1]]}
		C = [2]float64{VX[verts[2]], VY[verts[2]]}
		v0 = minus(C, A)
		v1 = minus(B, A)
		v2 = minus(P, A)
		dot00 := dot(v0, v0)
		dot01 := dot(v0, v1)
		dot02 := dot(v0, v2)
		dot11 := dot(v1, v1)
		dot12 := dot(v1, v2)
		invDenom := 1. / (dot00*dot11 - dot01*dot01)
		r = (dot11*dot02 - dot01*dot12) * invDenom
		s = (dot00*dot12 - dot01*dot02) * invDenom
		if r >= 0 && s >= 0 && (r+s) < 1. {
			ElementID = k
			return
		}
	}
	err := fmt.Errorf("unable to find point within elements")
	panic(err)
}

func (st *SODShockTube) interpolateFields(Q [4]utils.Matrix) {
	var (
		SolPts = utils.NewMatrix(st.DFR2D.SolutionElement.Np, 1)
	)
	for i, it := range st.InterpolationTarget {
		k := it.ElementNumber
		copy(SolPts.DataP, Q[0].Col(k).DataP)
		fM := it.InterpolationMatrix.Mul(SolPts)
		st.Rho[i] = fM.DataP[0]

		copy(SolPts.DataP, Q[1].Col(k).DataP)
		fM = it.InterpolationMatrix.Mul(SolPts)
		st.RhoU[i] = fM.DataP[0]

		copy(SolPts.DataP, Q[3].Col(k).DataP)
		fM = it.InterpolationMatrix.Mul(SolPts)
		st.E[i] = fM.DataP[0]
	}
}

func (st *SODShockTube) Plot(timeT float64, graphDelay time.Duration, Q [4]utils.Matrix) (iRho float64) {
	var (
		fmin, fmax float32
	)
	st.interpolateFields(Q)
	xmin, xmax := float32(0), float32(1)
	fmin, fmax = float32(-0.1), float32(2.6)
	st.plotOnce.Do(func() {
		st.chart = chart2d.NewChart2D(1920, 1280, xmin, xmax, fmin, fmax)
		st.colorMap = utils2.NewColorMap(-1, 1, 1)
		go st.chart.Plot()
	})
	pSeries := func(field []float64, name string, color float32, gl chart2d.GlyphType) {
		if err := st.chart.AddSeries(name, st.XLocations, field,
			gl, chart2d.Solid, st.colorMap.GetRGB(color)); err != nil {
			panic("unable to add graph series")
		}
	}
	pSeries(st.Rho, "Rho", -0.7, chart2d.NoGlyph)
	pSeries(st.RhoU, "RhoU", 0.0, chart2d.NoGlyph)
	pSeries(st.E, "Ener", 0.7, chart2d.NoGlyph)
	iRho = Euler1D.AddAnalyticSod(st.chart, st.colorMap, timeT)
	time.Sleep(graphDelay)
	return
}
