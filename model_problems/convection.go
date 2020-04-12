package model_problems

import (
	"fmt"
	"math"
	"sync"

	"gonum.org/v1/gonum/mat"

	"github.com/notargets/gocfd/utils"

	"github.com/notargets/gocfd/DG1D"

	"github.com/notargets/avs/chart2d"

	utils2 "github.com/notargets/avs/utils"
)

type Convection1D struct {
	// Input parameters
	a, CFL, FinalTime float64
	El                *DG1D.Elements1D
	RHSOnce           sync.Once
	UFlux             utils.Matrix
}

func NewConvection(a, CFL, FinalTime float64, Elements *DG1D.Elements1D) *Convection1D {
	return &Convection1D{
		a:         a,
		CFL:       CFL,
		FinalTime: FinalTime,
		El:        Elements,
	}
}

func (c *Convection1D) Run() {
	var (
		el   = c.El
		rk4a = []float64{
			0.0,
			-567301805773.0 / 1357537059087.0,
			-2404267990393.0 / 2016746695238.0,
			-3550918686646.0 / 2091501179385.0,
			-1275806237668.0 / 842570457699.0,
		}
		rk4b = []float64{
			1432997174477.0 / 9575080441755.0,
			5161836677717.0 / 13612068292357.0,
			1720146321549.0 / 2090206949498.0,
			3134564353537.0 / 4481467310338.0,
			2277821191437.0 / 14882151754819.0,
		}
		rk4c = []float64{
			0.0,
			1432997174477.0 / 9575080441755.0,
			2526269341429.0 / 6820363962896.0,
			2006345519317.0 / 3224310063776.0,
			2802321613138.0 / 2924317926251.0,
			1.,
		}
	)
	xmin := el.X.Row(1).Subtract(el.X.Row(0)).Apply(math.Abs).Min()
	dt := 0.5 * xmin * (c.CFL / c.a)
	Ns := math.Ceil(c.FinalTime / dt)
	dt = c.FinalTime / Ns
	Nsteps := int(Ns)
	fmt.Printf("Min Dist = %8.6f, dt = %8.6f, Nsteps = %d\n\n", xmin, dt, Nsteps)
	U := el.X.Copy().Apply(math.Sin)
	//fmt.Printf("U = \n%v\n", mat.Formatted(U, mat.Squeeze()))
	resid := utils.NewMatrix(el.Np, el.K)
	chart := chart2d.NewChart2D(1024, 768, 0, 20, -1, 1)
	colorMap := utils2.NewColorMap(-1, 1, 1)
	fmt.Printf("X data = \n%v\n", el.X.Transpose().RawMatrix().Data)
	chartName := "Advect1D"
	if err := chart.AddSeries(chartName,
		ToFloat32Slice(el.X.Transpose().RawMatrix().Data),
		ToFloat32Slice(U.Transpose().RawMatrix().Data),
		chart2d.NoGlyph, chart2d.Solid,
		colorMap.GetRGB(0)); err != nil {
		panic("unable to add graph series")
	}
	go chart.Plot()
	var time, timelocal float64
	for tstep := 0; tstep < Nsteps; tstep++ {
		for INTRK := 0; INTRK < 5; INTRK++ {
			timelocal = time + dt*rk4c[INTRK]
			RHSU := c.RHS(U, timelocal)
			// resid = rk4a(INTRK) * resid + dt * rhsu;
			resid.Scale(rk4a[INTRK]).Add(RHSU.Scale(dt))
			// u += rk4b(INTRK) * resid;
			U.Add(resid.Copy().Scale(rk4b[INTRK]))
		}
		time += dt
		if tstep%5000 == 0 {
			if err := chart.AddSeries(chartName,
				ToFloat32Slice(el.X.Transpose().RawMatrix().Data),
				ToFloat32Slice(U.Transpose().RawMatrix().Data),
				chart2d.NoGlyph, chart2d.Solid,
				colorMap.GetRGB(0)); err != nil {
				panic("unable to add graph series")
			}
			fmt.Printf("time = %8.4f, max_resid[%d] = %8.4f, umin = %8.4f, umax = %8.4f\n", time, tstep, resid.Max(), U.Col(0).Min(), U.Col(0).Max())
		}
	}
	fmt.Printf("U = \n%v\n", mat.Formatted(U, mat.Squeeze()))
}

func (c *Convection1D) RHS(U utils.Matrix, time float64) (RHSU utils.Matrix) {
	var (
		uin   float64
		alpha = 1.0 // flux splitting parameter, 0 is full upwinding
		el    = c.El
	)
	c.RHSOnce.Do(func() {
		aNX := el.NX.Copy().Scale(c.a)
		aNXabs := aNX.Copy().Apply(math.Abs).Scale(1. - alpha)
		c.UFlux = aNX.Subtract(aNXabs)
	})
	// Face fluxes
	// du = (u(vmapM)-u(vmapP)).dm(a*nx-(1.-alpha)*abs(a*nx))/2.;
	duNr := el.Nfp * el.NFaces
	duNc := el.K
	dU := U.Subset(el.VmapM, duNr, duNc).Subtract(U.Subset(el.VmapP, duNr, duNc)).ElementMultiply(c.UFlux).Scale(0.5)

	// Boundaries
	// Inflow boundary
	// du(mapI) = (u(vmapI)-uin).dm(a*nx(mapI)-(1.-alpha)*abs(a*nx(mapI)))/2.;
	uin = -math.Sin(c.a * time)
	dU.Assign(el.MapI, U.Subset(el.VmapI, duNr, duNc).AddScalar(-uin).ElementMultiply(c.UFlux.Subset(el.MapI, duNr, duNc)).Scale(0.5))
	dU.AssignScalar(el.MapO, 0)

	// rhsu = -a*rx.dm(Dr*u) + LIFT*(Fscale.dm(du));
	// Important: must change the order from Fscale.dm(du) to du.dm(Fscale) here because the dm overwrites the target
	RHSU = el.Rx.Copy().Scale(-c.a).ElementMultiply(el.Dr.Mul(U)).Add(el.LIFT.Mul(dU.ElementMultiply(el.FScale)))
	return
}

func ToFloat32Slice(A []float64) (R []float32) {
	R = make([]float32, len(A))
	for i, val := range A {
		R[i] = float32(val)
	}
	return R
}
