package model_problems

import (
	"fmt"
	"math"
	"sync"
	"time"

	"gonum.org/v1/gonum/mat"

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
	State           *FieldState
	Rho, RhoU, Ener utils.Matrix
}

type FieldState struct {
	Gamma                                        float64
	U, RhoU2, Pres, CVel, LM, RhoF, RhoUF, EnerF utils.Matrix
}

func NewFieldState() (fs *FieldState) {
	return &FieldState{}
}

func (fs *FieldState) Update(Rho, RhoU, Ener utils.Matrix) {
	fs.U = RhoU.Copy().ElDiv(Rho)                                // Velocity
	fs.RhoU2 = RhoU.Copy().POW(2).Scale(0.5).ElDiv(Rho)          // 1/2 * Rho * U^2
	fs.Pres = Ener.Copy().Subtract(fs.RhoU2).Scale(fs.Gamma - 1) // Pressure
	fs.CVel = fs.Pres.Copy().ElDiv(Rho).Scale(fs.Gamma).Apply(math.Sqrt)
	fs.LM = fs.U.Copy().Apply(math.Abs).Add(fs.CVel)
	fs.RhoF = RhoU.Copy()
	fs.RhoUF = fs.RhoU2.Scale(2).Add(fs.Pres)
	fs.EnerF = Ener.Copy().Add(fs.Pres).ElMul(RhoU.Copy().ElDiv(Rho))
}

func (fs *FieldState) Print() {
	fmt.Printf("U = \n%v\n", mat.Formatted(fs.U, mat.Squeeze()))
	fmt.Printf("RhoU2 = \n%v\n", mat.Formatted(fs.RhoU2, mat.Squeeze()))
	fmt.Printf("Pres = \n%v\n", mat.Formatted(fs.Pres, mat.Squeeze()))
	fmt.Printf("CVel = \n%v\n", mat.Formatted(fs.CVel, mat.Squeeze()))
	fmt.Printf("LM = \n%v\n", mat.Formatted(fs.LM, mat.Squeeze()))
	fmt.Printf("RhoF = \n%v\n", mat.Formatted(fs.RhoF, mat.Squeeze()))
	fmt.Printf("RhoUF = \n%v\n", mat.Formatted(fs.RhoUF, mat.Squeeze()))
	fmt.Printf("EnerF = \n%v\n", mat.Formatted(fs.EnerF, mat.Squeeze()))
}

func NewEuler1D(CFL, FinalTime float64, N, K int) (c *Euler1D) {
	VX, EToV := DG1D.SimpleMesh1D(0, 1, K)
	c = &Euler1D{
		CFL:       CFL,
		State:     NewFieldState(),
		FinalTime: FinalTime,
		El:        DG1D.NewElements1D(N, VX, EToV),
	}
	c.State.Gamma = 1.4
	c.InitializeSOD()
	return
}

func (c *Euler1D) InitializeSOD() {
	var (
		/*
			In                = NewStateP(c.Gamma, 1, 0, 1)
			Out               = NewStateP(c.Gamma, 0.125, 0, 0.1)
		*/
		el                = c.El
		MassMatrix, VtInv utils.Matrix
		err               error
		npOnes            = utils.NewVectorConstant(el.Np, 1)
		s                 = c.State
	)
	if VtInv, err = el.V.Transpose().Inverse(); err != nil {
		panic(err)
	}
	MassMatrix = VtInv.Mul(el.Vinv)
	CellCenterRValues := MassMatrix.Mul(el.X).SumCols().Scale(0.5)
	cx := npOnes.Outer(CellCenterRValues)
	leftHalf := cx.Find(utils.Less, 0.5, false)
	rightHalf := cx.Find(utils.GreaterOrEqual, 0.5, false)
	// Initialize field variables
	c.Rho = utils.NewMatrix(el.Np, el.K)
	c.Rho.AssignScalar(leftHalf, 1)
	c.Rho.AssignScalar(rightHalf, 0.125)
	c.RhoU = utils.NewMatrix(el.Np, el.K)
	c.RhoU.Scale(0)
	rDiv := 1. / (s.Gamma - 1.)
	c.Ener = utils.NewMatrix(el.Np, el.K)
	c.Ener.AssignScalar(leftHalf, rDiv)
	c.Ener.AssignScalar(rightHalf, 0.1*rDiv)
}

func (c *Euler1D) Run(showGraph bool, graphDelay ...time.Duration) {
	var (
		chart                          *chart2d.Chart2D
		colorMap                       *utils2.ColorMap
		chartRho, chartRhoU, chartEner string
		el                             = c.El
		logFrequency                   = 50
		s                              = c.State
	)
	if showGraph {
		chart = chart2d.NewChart2D(1920, 1280, float32(el.X.Min()), float32(el.X.Max()), -.5, 3)
		colorMap = utils2.NewColorMap(-1, 1, 1)
		chartRho = "Density"
		chartRhoU = "Momentum"
		chartEner = "Energy"
		go chart.Plot()
	}
	xmin := el.X.Row(1).Subtract(el.X.Row(0)).Apply(math.Abs).Min()
	s.Update(c.Rho, c.RhoU, c.Ener)
	dt := c.CalculateDT(xmin)
	c.Rho = el.SlopeLimitN(c.Rho, 0)
	c.RhoU = el.SlopeLimitN(c.RhoU, 0)
	c.Ener = el.SlopeLimitN(c.Ener, 0)

	fmt.Printf("FinalTime = %8.4f, dt = %8.6f\n", c.FinalTime, dt)
	var Time float64
	var tstep int
	for c.FinalTime > 0 {
		dt = c.CalculateDT(xmin)
		fmt.Printf("LAL here 1, dt = %v\n", dt)
		if showGraph {
			if err := chart.AddSeries(chartRho, el.X.Transpose().RawMatrix().Data, c.Rho.Transpose().RawMatrix().Data,
				chart2d.XGlyph, chart2d.Solid, colorMap.GetRGB(0)); err != nil {
				panic("unable to add graph series")
			}
			if err := chart.AddSeries(chartRhoU, el.X.Transpose().RawMatrix().Data, c.RhoU.Transpose().RawMatrix().Data,
				chart2d.XGlyph, chart2d.Solid, colorMap.GetRGB(0.7)); err != nil {
				panic("unable to add graph series")
			}
			if err := chart.AddSeries(chartEner, el.X.Transpose().RawMatrix().Data, c.Ener.Transpose().RawMatrix().Data,
				chart2d.XGlyph, chart2d.Solid, colorMap.GetRGB(-0.7)); err != nil {
				panic("unable to add graph series")
			}
			if len(graphDelay) != 0 {
				time.Sleep(graphDelay[0])
			}
		}
		/*
			Third Order Runge-Kutta time advancement
		*/
		// SSP RK Stage 1
		rhsRho, rhsRhoU, rhsEner := c.RHS(c.Rho, c.RhoU, c.Ener)
		rho1 := c.Rho.Copy().Add(rhsRho.Scale(dt))
		rhou1 := c.RhoU.Copy().Add(rhsRhoU.Scale(dt))
		ener1 := c.Ener.Copy().Add(rhsEner.Scale(dt))
		/*
			fmt.Printf("rhsRho = \n%v\n", mat.Formatted(rhsRho, mat.Squeeze()))
			fmt.Printf("rhsRhoU = \n%v\n", mat.Formatted(rhsRhoU, mat.Squeeze()))
			fmt.Printf("rhsEner = \n%v\n", mat.Formatted(rhsEner, mat.Squeeze()))
			fmt.Printf("rho1 = \n%v\n", mat.Formatted(rho1, mat.Squeeze()))
			fmt.Printf("rhou1 = \n%v\n", mat.Formatted(rhou1, mat.Squeeze()))
			fmt.Printf("ener1 = \n%v\n", mat.Formatted(ener1, mat.Squeeze()))
			os.Exit(1)
		*/
		// Slope Limit the fields
		el.SlopeLimitN(rho1, 20)
		el.SlopeLimitN(rhou1, 20)
		el.SlopeLimitN(ener1, 20)
		// SSP RK Stage 2
		rhsRho, rhsRhoU, rhsEner = c.RHS(rho1, rhou1, ener1)
		rho2 := c.Rho.Copy().Scale(3).Add(rho1).Add(rhsRho.Scale(dt)).Scale(0.25)
		rhou2 := c.RhoU.Copy().Scale(3).Add(rhou1).Add(rhsRhoU.Scale(dt)).Scale(0.25)
		ener2 := c.Ener.Copy().Scale(3).Add(ener1).Add(rhsEner.Scale(dt)).Scale(0.25)
		// Slope Limit the fields
		el.SlopeLimitN(rho2, 20)
		el.SlopeLimitN(rhou2, 20)
		el.SlopeLimitN(ener2, 20)
		// SSP RK Stage 3
		rhsRho, rhsRhoU, rhsEner = c.RHS(rho2, rhou2, ener2)
		c.Rho.Add(rho2.Scale(2)).Add(rhsRho.Scale(2 * dt)).Scale(1 / 3)
		c.RhoU.Add(rhou2.Scale(2)).Add(rhsRhoU.Scale(2 * dt)).Scale(1 / 3)
		c.Ener.Add(ener2.Scale(2)).Add(rhsEner.Scale(2 * dt)).Scale(1 / 3)
		// Slope Limit the fields
		el.SlopeLimitN(c.Rho, 20)
		el.SlopeLimitN(c.RhoU, 20)
		el.SlopeLimitN(c.Ener, 20)

		Time += dt
		c.FinalTime -= dt
		tstep++
		if tstep%logFrequency == 0 {
			fmt.Printf("Time = %8.4f, max_resid[%d] = %8.4f, emin = %8.6f, emax = %8.6f\n", Time, tstep, rhsEner.Max(), c.Ener.Min(), c.Ener.Max())
		}
	}
	return
}

func (c *Euler1D) CalculateDT(xmin float64) (dt float64) {
	var (
		s = c.State
	)
	// min(xmin ./ (abs(U) +C))
	Factor := s.U.Copy().Apply(math.Abs).Add(s.CVel).Apply(func(val float64) float64 { return xmin / val })
	dt = c.CFL * Factor.Min()
	dt = dt / 10000
	return
}

func (c *Euler1D) RHS(Rho, RhoU, Ener utils.Matrix) (rhsRho, rhsRhoU, rhsEner utils.Matrix) {
	var (
		el       = c.El
		nrF, ncF = el.Nfp * el.NFaces, el.K
		s        = c.State
		// Sod's problem: Shock tube with jump in middle
		In                                                 = NewStateP(s.Gamma, 1, 0, 1)
		Out                                                = NewStateP(s.Gamma, 0.125, 0, 0.1)
		dRho, dRhoU, dEner, dRhoF, dRhoUF, dEnerF, LFcDiv2 utils.Matrix
	)
	s.Update(Rho, RhoU, Ener)
	// Face jumps in primary and flux variables
	dRho = Rho.Subset(el.VmapM, nrF, ncF).Subtract(Rho.Subset(el.VmapP, nrF, ncF))
	dRhoU = RhoU.Subset(el.VmapM, nrF, ncF).Subtract(RhoU.Subset(el.VmapP, nrF, ncF))
	dEner = Ener.Subset(el.VmapM, nrF, ncF).Subtract(Ener.Subset(el.VmapP, nrF, ncF))

	dRhoF = s.RhoF.Subset(el.VmapM, nrF, ncF).Subtract(s.RhoF.Subset(el.VmapP, nrF, ncF))
	dRhoUF = s.RhoUF.Subset(el.VmapM, nrF, ncF).Subtract(s.RhoUF.Subset(el.VmapP, nrF, ncF))
	dEnerF = s.EnerF.Subset(el.VmapM, nrF, ncF).Subtract(s.EnerF.Subset(el.VmapP, nrF, ncF))
	// Lax-Friedrichs flux component is always used divided by 2, so we pre-scale it
	LFcDiv2 = s.LM.Subset(el.VmapM, nrF, ncF).Apply2(math.Max, s.EnerF.Subset(el.VmapP, nrF, ncF)).Scale(0.5)

	// Compute fluxes at interfaces
	dRhoF.ElMul(el.NX).Scale(0.5).Subtract(LFcDiv2.Copy().ElMul(dRho))
	dRhoUF.ElMul(el.NX).Scale(0.5).Subtract(LFcDiv2.Copy().ElMul(dRhoU))
	dEnerF.ElMul(el.NX).Scale(0.5).Subtract(LFcDiv2.Copy().ElMul(dEner))

	// Boundary conditions for Sod's problem
	// Inflow
	lmI := s.LM.SubsetVector(el.VmapI).Scale(0.5)
	nxI := el.NX.SubsetVector(el.MapI)
	dRhoFIn := nxI.Outer(s.RhoF.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.RhoF)))
	dRhoFIn.Subtract(lmI.Outer(Rho.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.Rho))))
	dRhoF.AssignVector(el.MapI, dRhoFIn)
	dRhoUFIn := nxI.Outer(s.RhoUF.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.RhoUF)))
	dRhoUFIn.Subtract(lmI.Outer(RhoU.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.RhoU))))
	dRhoUF.AssignVector(el.MapI, dRhoUFIn)
	dEnerFIn := nxI.Outer(s.EnerF.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.EnerF)))
	dEnerFIn.Subtract(lmI.Outer(Ener.SubsetVector(el.VmapI).Subtract(utils.NewVectorConstant(len(el.VmapI), In.Ener))))
	dEnerF.AssignVector(el.MapI, dEnerFIn)
	// Outflow
	lmO := s.LM.SubsetVector(el.VmapO).Scale(0.5)
	nxO := el.NX.SubsetVector(el.MapO)
	dRhoFOut := nxO.Outer(s.RhoF.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.RhoF)))
	dRhoFOut.Subtract(lmO.Outer(Rho.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.Rho))))
	dRhoF.AssignVector(el.MapO, dRhoFOut)
	dRhoUFOut := nxO.Outer(s.RhoUF.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.RhoUF)))
	dRhoUFOut.Subtract(lmO.Outer(RhoU.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.RhoU))))
	dRhoUF.AssignVector(el.MapO, dRhoUFOut)
	dEnerFOut := nxO.Outer(s.EnerF.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.EnerF)))
	dEnerFOut.Subtract(lmO.Outer(Ener.SubsetVector(el.VmapO).Subtract(utils.NewVectorConstant(len(el.VmapO), Out.Ener))))
	dEnerF.AssignVector(el.MapO, dEnerFOut)

	// RHS Computation
	rhsRho = el.Rx.Copy().Scale(-1).ElMul(el.Dr.Mul(s.RhoF)).Add(el.LIFT.Mul(dRhoF.ElMul(el.FScale)))
	rhsRhoU = el.Rx.Copy().Scale(-1).ElMul(el.Dr.Mul(s.RhoUF)).Add(el.LIFT.Mul(dRhoUF.ElMul(el.FScale)))
	rhsEner = el.Rx.Copy().Scale(-1).ElMul(el.Dr.Mul(s.EnerF)).Add(el.LIFT.Mul(dEnerF.ElMul(el.FScale)))

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
