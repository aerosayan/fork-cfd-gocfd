package model_problems

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/notargets/avs/chart2d"
	utils2 "github.com/notargets/avs/utils"

	"github.com/notargets/gocfd/DG1D"
	"github.com/notargets/gocfd/utils"
)

type Euler1D struct {
	// Input parameters
	CFL, FinalTime  float64
	El              *DG1D.Elements1D
	RHSOnce         sync.Once
	Rho, RhoU, Ener utils.Matrix
}

func NewEuler1D(CFL, FinalTime float64, N, K int) (c *Euler1D) {
	VX, EToV := DG1D.SimpleMesh1D(-2, 2, K)
	c = &Euler1D{
		CFL:       CFL,
		FinalTime: FinalTime,
		El:        DG1D.NewElements1D(N, VX, EToV),
	}
	return
}

func (c *Euler1D) Run(showGraph bool, graphDelay ...time.Duration) {
	var (
		chart                          *chart2d.Chart2D
		colorMap                       *utils2.ColorMap
		chartRho, chartRhoU, chartEner string
		el                             = c.El
		resRho                         = utils.NewMatrix(el.Np, el.K)
		resRhoU                        = utils.NewMatrix(el.Np, el.K)
		resEner                        = utils.NewMatrix(el.Np, el.K)
		logFrequency                   = 50
	)
	_, _, _ = resRho, resRhoU, resEner
	if showGraph {
		chart = chart2d.NewChart2D(1920, 1280, float32(el.X.Min()), float32(el.X.Max()), -1, 1)
		colorMap = utils2.NewColorMap(-1, 1, 1)
		chartRho = "Density"
		chartRhoU = "Momentum"
		chartEner = "Energy"
		go chart.Plot()
	}
	xmin := el.X.Row(1).Subtract(el.X.Row(0)).Apply(math.Abs).Min()
	dt := xmin * c.CFL
	Nsteps := int(math.Ceil(c.FinalTime / dt))
	dt = c.FinalTime / float64(Nsteps)
	fmt.Printf("FinalTime = %8.4f, Nsteps = %d, dt = %8.6f\n", c.FinalTime, Nsteps, dt)

	var Time float64
	for tstep := 0; tstep < Nsteps; tstep++ {
		if showGraph {
			if err := chart.AddSeries(chartRho, el.X.Transpose().RawMatrix().Data, c.Rho.Transpose().RawMatrix().Data,
				chart2d.CrossGlyph, chart2d.Dashed, colorMap.GetRGB(0)); err != nil {
				panic("unable to add graph series")
			}
			if err := chart.AddSeries(chartRhoU, el.X.Transpose().RawMatrix().Data, c.RhoU.Transpose().RawMatrix().Data,
				chart2d.CrossGlyph, chart2d.Dashed, colorMap.GetRGB(0.7)); err != nil {
				panic("unable to add graph series")
			}
			if err := chart.AddSeries(chartEner, el.X.Transpose().RawMatrix().Data, c.Ener.Transpose().RawMatrix().Data,
				chart2d.CrossGlyph, chart2d.Dashed, colorMap.GetRGB(-0.7)); err != nil {
				panic("unable to add graph series")
			}
			if len(graphDelay) != 0 {
				time.Sleep(graphDelay[0])
			}
		}
		for INTRK := 0; INTRK < 5; INTRK++ {
			rhsRho, rhsRhoU, rhsEner := c.RHS()
			_, _, _ = rhsRho, rhsRhoU, rhsEner
		}
		Time += dt
		if tstep%logFrequency == 0 {
			fmt.Printf("Time = %8.4f, max_resid[%d] = %8.4f, emin = %8.6f, emax = %8.6f\n", Time, tstep, resEner.Max(), c.Ener.Min(), c.Ener.Max())
		}
	}
	return
}

type State struct {
	Gamma, Rho, RhoU, Ener float64
	RhoF, RhoUF, EnerF     float64
}

func NewState(gamma, rho, rhoU, ener float64) (s *State) {
	p := ener * (gamma - 1.)
	q := 0.5 * utils.POW(rhoU, 2) / rho
	u := rhoU / rho
	return &State{
		Gamma: gamma,
		Rho:   rho,
		RhoU:  rhoU,
		Ener:  ener,
		RhoF:  rhoU,
		RhoUF: 2*q + p,
		EnerF: (ener + q + p) * u,
	}
}

func NewStateP(gamma, rho, rhoU, p float64) *State {
	ener := p / (gamma - 1.)
	return NewState(gamma, rho, rhoU, ener)
}

func (c *Euler1D) RHS() (rhsRho, rhsRhoU, rhsEner utils.Matrix) {
	var (
		el       = c.El
		nrF, ncF = el.Nfp * el.NFaces, el.K
		gamma    = 1.4
		//TODO: Remove COPY() by promoting a state struct allocated above, with an UPDATE() method
		U     = c.RhoU.Copy().ElDiv(c.Rho)                     // Velocity
		RhoU2 = c.RhoU.Copy().POW(2).Scale(0.5).ElDiv(c.Rho)   // 1/2 * Rho * U^2
		Pres  = c.Ener.Copy().Subtract(RhoU2).Scale(gamma - 1) // Pressure
		CVel  = Pres.Copy().ElDiv(c.Rho).Scale(gamma).Apply(math.Sqrt)
		LM    = U.Copy().Apply(math.Abs).Add(CVel)
		RhoF  = c.RhoU.Copy()
		RhoUF = RhoU2.Scale(2).Add(Pres)
		EnerF = c.Ener.Copy().Add(Pres).ElDiv(c.RhoU.Copy().ElDiv(c.Rho))
		// Face jumps in primary and flux variables
		dRho   = c.Rho.Subset(el.VmapM, nrF, ncF).Subtract(c.Rho.Subset(el.VmapP, nrF, ncF))
		dRhoU  = c.RhoU.Subset(el.VmapM, nrF, ncF).Subtract(c.RhoU.Subset(el.VmapP, nrF, ncF))
		dEner  = c.Ener.Subset(el.VmapM, nrF, ncF).Subtract(c.Ener.Subset(el.VmapP, nrF, ncF))
		dRhoF  = RhoF.Subset(el.VmapM, nrF, ncF).Subtract(RhoF.Subset(el.VmapP, nrF, ncF))
		dRhoUF = RhoUF.Subset(el.VmapM, nrF, ncF).Subtract(RhoUF.Subset(el.VmapP, nrF, ncF))
		dEnerF = EnerF.Subset(el.VmapM, nrF, ncF).Subtract(EnerF.Subset(el.VmapP, nrF, ncF))
		// Lax-Friedrichs flux component is always used divided by 2, so we pre-scale it
		LFcDiv2 = LM.Subset(el.VmapM, nrF, ncF).Apply2(math.Max, EnerF.Subset(el.VmapP, nrF, ncF)).Scale(0.5)
		// Sod's problem: Shock tube with jump in middle
		In  = NewStateP(gamma, 1, 0, 1)
		Out = NewStateP(gamma, 0.125, 0, 0.1)
	)
	// Compute fluxes at interfaces
	dRhoF.ElMul(el.NX).Scale(0.5).Subtract(LFcDiv2.Copy().ElMul(dRho))
	dRhoUF.ElMul(el.NX).Scale(0.5).Subtract(LFcDiv2.Copy().ElMul(dRhoU))
	dEnerF.ElMul(el.NX).Scale(0.5).Subtract(LFcDiv2.Copy().ElMul(dEner))

	// Boundary conditions for Sod's problem
	// Inflow
	lmI := LM.SubsetVector(el.VmapI).Scale(0.5)
	nxI := el.NX.SubsetVector(el.MapI)
	dRhoF.Assign(el.MapI, nxI.Outer(RhoF.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.RhoF))))
	dRhoF.Subtract(lmI.Outer(c.Rho.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.Rho))))
	dRhoUF.Assign(el.MapI, nxI.Outer(RhoUF.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.RhoUF))))
	dRhoUF.Subtract(lmI.Outer(c.RhoU.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.RhoU))))
	dEnerF.Assign(el.MapI, nxI.Outer(EnerF.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.EnerF))))
	dEnerF.Subtract(lmI.Outer(c.Ener.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.Ener))))
	// Outflow
	lmO := LM.SubsetVector(el.VmapO).Scale(0.5)
	nxO := el.NX.SubsetVector(el.MapO)
	dRhoF.Assign(el.MapO, nxO.Outer(RhoF.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.RhoF))))
	dRhoF.Subtract(lmO.Outer(c.Rho.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.Rho))))
	dRhoUF.Assign(el.MapO, nxO.Outer(RhoUF.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.RhoUF))))
	dRhoUF.Subtract(lmO.Outer(c.RhoU.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.RhoU))))
	dEnerF.Assign(el.MapO, nxO.Outer(EnerF.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.EnerF))))
	dEnerF.Subtract(lmO.Outer(c.Ener.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.Ener))))

	// RHS Computation
	rhsRho = el.Rx.Copy().Scale(-1).ElMul(el.Dr.Mul(RhoF)).Add(el.LIFT.Mul(dRhoF.ElMul(el.FScale)))
	rhsRhoU = el.Rx.Copy().Scale(-1).ElMul(el.Dr.Mul(RhoUF)).Add(el.LIFT.Mul(dRhoUF.ElMul(el.FScale)))
	rhsEner = el.Rx.Copy().Scale(-1).ElMul(el.Dr.Mul(EnerF)).Add(el.LIFT.Mul(dEnerF.ElMul(el.FScale)))

	return
}