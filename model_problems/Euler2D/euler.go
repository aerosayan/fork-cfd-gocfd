package Euler2D

import (
	"fmt"
	"math"
	"sync"
	"syscall"
	"time"

	"github.com/notargets/gocfd/types"

	"github.com/notargets/gocfd/DG2D"

	"github.com/notargets/gocfd/utils"

	"github.com/pkg/profile"
)

/*
	In the DFR scheme, we have two sets of points:
		- Solution points (inside element)
		- Flux Points (on element faces)
	The flux element operates on both sets of points, the solution element only operates on the internal solution points
*/
type Euler struct {
	// Input parameters
	MeshFile          string
	CFL, FinalTime    float64
	FS                *FreeStream
	dfr               *DG2D.DFR2D
	chart             ChartState
	profile           bool // Generate a CPU profile of the solver
	FluxCalcAlgo      FluxType
	Case              InitType
	AnalyticSolution  ExactState
	FluxCalcMock      func(rho, rhoU, rhoV, E float64) (Fx, Fy [4]float64) // For testing
	SortedEdgeKeys    []EdgeKeySlice                                       // Buckets, one for each parallel partition
	Partitions        *PartitionMap                                        // mapping of elements into bins for parallelism
	LocalTimeStepping bool
	MaxIterations     int
	// Below are partitioned by K (elements) in the first slice
	Q                    [][4]utils.Matrix // Sharded solution variables, stored at solution point locations, Np_solution x K
	SolutionX, SolutionY []utils.Matrix
	ShockFinder          *ModeAliasShockFinder
	Limiter              *SolutionLimiter
	Dissipation          *ScalarDissipation
	// Edge number mapped quantities, i.e. Face Normal Flux
	EdgeStore *EdgeValueStorage
}

func NewEuler(ip *InputParameters, meshFile string, ProcLimit int, plotMesh, verbose, profile bool) (c *Euler) {
	c = &Euler{
		MeshFile:          meshFile,
		CFL:               ip.CFL,
		FinalTime:         ip.FinalTime,
		FluxCalcAlgo:      NewFluxType(ip.FluxType),
		Case:              NewInitType(ip.InitType),
		LocalTimeStepping: ip.LocalTimeStepping,
		MaxIterations:     ip.MaxIterations,
		FS:                NewFreeStream(ip.Minf, ip.Gamma, ip.Alpha),
		profile:           profile,
	}
	c.FluxCalcMock = c.FluxCalcBase

	if len(meshFile) == 0 {
		return
	}

	// Read mesh file, initialize geometry and finite elements
	c.dfr = DG2D.NewDFR2D(ip.PolynomialOrder, plotMesh, meshFile)

	c.SetParallelDegree(ProcLimit, c.dfr.K) // Must occur after determining the number of elements
	c.PartitionEdgesByK()                   // Setup the key for edge calculations, useful for parallelizing the process

	// Allocate Normal flux storage and indices
	c.EdgeStore = c.NewEdgeStorage()

	c.InitializeSolution(verbose)

	// Allocate a solution limiter
	c.Limiter = NewSolutionLimiter(NewLimiterType(ip.Limiter), ip.Kappa, c.dfr, c.Partitions, c.FS)

	// Initiate Artificial Dissipation
	c.Dissipation = NewScalarDissipation(ip.Kappa, c.dfr, c.Partitions)

	if verbose {
		fmt.Printf("Euler Equations in 2 Dimensions\n")
		fmt.Printf("Using %d go routines in parallel\n", c.Partitions.ParallelDegree)
		fmt.Printf("Solving %s\n", c.Case.Print())
		switch c.Case {
		case FREESTREAM:
			fmt.Printf("Mach Infinity = %8.5f, Angle of Attack = %8.5f\n", ip.Minf, ip.Alpha)
		}
		fmt.Printf("Flux Algorithm: [%s] using Limiter: [%s]\n", c.FluxCalcAlgo.Print(), c.Limiter.limiterType.Print())
		fmt.Printf("Artificial Dissipation: Kappa = [%5.3f]\n", c.Dissipation.Kappa)
		fmt.Printf("CFL = %8.4f, Polynomial Degree N = %d (1 is linear), Num Elements K = %d\n\n\n",
			ip.CFL, ip.PolynomialOrder, c.dfr.K)
	}
	return
}

func (c *Euler) Solve(pm *PlotMeta) {
	var (
		FinalTime = c.FinalTime
		steps     int
		finished  bool
		plotQ     = pm.Plot
	)
	if c.profile {
		defer profile.Start().Stop()
	}

	c.PrintInitialization(FinalTime)

	rk := c.NewRungeKuttaSSP()

	elapsed := time.Duration(0)
	var start time.Time
	for !finished {
		start = time.Now()
		rk.Step(c)
		elapsed += time.Now().Sub(start)
		steps++
		rk.Time += rk.GlobalDT
		finished = c.CheckIfFinished(rk.Time, FinalTime, steps)
		if finished || steps%pm.StepsBeforePlot == 0 || steps == 1 {
			var printMem bool
			if steps%100 == 0 {
				printMem = true
			}
			c.PrintUpdate(rk.Time, rk.GlobalDT, steps, c.Q, rk.Residual, plotQ, pm, printMem)
		}
	}
	c.PrintFinal(elapsed, steps)
}

type RungeKutta4SSP struct {
	Jdet, Jinv           []utils.Matrix    // Sharded mesh Jacobian and inverse transform
	RHSQ, Q_Face         [][4]utils.Matrix // State used for matrix multiplies within the time step algorithm
	Q1, Q2, Q3           [][4]utils.Matrix // Intermediate solution state
	Residual             [][4]utils.Matrix // Used for reporting, aliased to Q1
	F_RT_DOF             [][4]utils.Matrix // Normal flux used for divergence
	DT                   []utils.Matrix    // Local time step storage
	MaxWaveSpeed         []float64         // Shard max wavespeed
	GlobalDT, Time       float64
	Kmax                 []int          // Local element count (dimension: Kmax[ParallelDegree])
	NpInt, Nedge, NpFlux int            // Number of points in solution, edge and flux total
	EdgeQ1, EdgeQ2       [][][4]float64 // Sharded local working memory, dimensions Nedge
}

func (c *Euler) NewRungeKuttaSSP() (rk *RungeKutta4SSP) {
	var (
		pm   = c.Partitions
		NPar = pm.ParallelDegree
	)
	rk = &RungeKutta4SSP{
		Jdet:         c.ShardByKTranspose(c.dfr.Jdet),
		Jinv:         c.ShardByKTranspose(c.dfr.Jinv),
		RHSQ:         make([][4]utils.Matrix, NPar),
		Q_Face:       make([][4]utils.Matrix, NPar),
		Q1:           make([][4]utils.Matrix, NPar),
		Q2:           make([][4]utils.Matrix, NPar),
		Q3:           make([][4]utils.Matrix, NPar),
		Residual:     make([][4]utils.Matrix, NPar),
		F_RT_DOF:     make([][4]utils.Matrix, NPar),
		DT:           make([]utils.Matrix, NPar),
		MaxWaveSpeed: make([]float64, NPar),
		Kmax:         make([]int, NPar),
		NpInt:        c.dfr.SolutionElement.Np,
		Nedge:        c.dfr.FluxElement.Nedge,
		NpFlux:       c.dfr.FluxElement.Np,
		EdgeQ1:       make([][][4]float64, NPar),
		EdgeQ2:       make([][][4]float64, NPar),
	}
	// Initialize memory for RHS
	for np := 0; np < NPar; np++ {
		rk.Kmax[np] = pm.GetBucketDimension(np)
		for n := 0; n < 4; n++ {
			rk.Q1[np][n] = utils.NewMatrix(rk.NpInt, rk.Kmax[np])
			rk.Q2[np][n] = utils.NewMatrix(rk.NpInt, rk.Kmax[np])
			rk.Q3[np][n] = utils.NewMatrix(rk.NpInt, rk.Kmax[np])
			rk.RHSQ[np][n] = utils.NewMatrix(rk.NpInt, rk.Kmax[np])
			rk.F_RT_DOF[np][n] = utils.NewMatrix(rk.NpFlux, rk.Kmax[np])
			rk.Q_Face[np][n] = utils.NewMatrix(rk.Nedge*3, rk.Kmax[np])
		}
		rk.DT[np] = utils.NewMatrix(rk.NpInt, rk.Kmax[np])
		rk.EdgeQ1[np] = make([][4]float64, rk.Nedge)
		rk.EdgeQ2[np] = make([][4]float64, rk.Nedge)
	}
	rk.Residual = rk.Q1
	return
}

func (rk *RungeKutta4SSP) Step(c *Euler) {
	/*
		This is the controller thread - it manages and synchronizes the workers
		NOTE: Any CPU work done in this thread makes the workers wait - be careful!
	*/
	var (
		pm       = c.Partitions
		NP       = pm.ParallelDegree
		wg       = sync.WaitGroup{}
		QCurrent [][4]utils.Matrix
	)
	for currentStep := 0; currentStep < 17; currentStep++ {
		// Workers are blocked below here until the StepWorker section - make sure significant work done here is abs necessary!
		switch {
		case currentStep == 1:
			// After step 0, we need to consolidate the local max wavespeeds to calculate global dt
			if !c.LocalTimeStepping {
				rk.calculateGlobalDT(c)
			}
		case currentStep%4 == 1: // pre-time advancement steps
			switch currentStep / 2 {
			case 0:
				QCurrent = c.Q
			case 1:
				QCurrent = rk.Q1
			case 2:
				QCurrent = rk.Q2
			case 3:
				QCurrent = rk.Q3
			}
			c.Dissipation.CalculateElementViscosity(QCurrent)
		}
		// Advance the workers one step by sending the "go" message. They are blocked until they receive this.
		for np := 0; np < NP; np++ {
			wg.Add(1)
			go rk.StepWorker(c, np, &wg, currentStep)
		}
		wg.Wait()
	}
}

func (rk *RungeKutta4SSP) StepWorker(c *Euler, myThread int, wg *sync.WaitGroup, subStep int) {
	var (
		Np                           = rk.NpInt
		dT                           float64
		contLevel                    = C0
		Q0                           = c.Q[myThread]
		Kmax, Jdet, Jinv, F_RT_DOF   = rk.Kmax[myThread], rk.Jdet[myThread], rk.Jinv[myThread], rk.F_RT_DOF[myThread]
		DT, Q_Face, Q1, Q2, Q3, RHSQ = rk.DT[myThread], rk.Q_Face[myThread], rk.Q1[myThread], rk.Q2[myThread], rk.Q3[myThread], rk.RHSQ[myThread]
		Residual                     = rk.Residual[myThread]
		EdgeQ1, EdgeQ2               = rk.EdgeQ1[myThread], rk.EdgeQ2[myThread]
		SortedEdgeKeys               = c.SortedEdgeKeys[myThread]
	)
	//defer func() { fmt.Printf("thread %d finished with step %d\n", myThread, subStep); wg.Done() }()
	defer wg.Done()
	switch subStep {
	case 0:
		if c.LocalTimeStepping {
			// Setup local time stepping
			for k := 0; k < Kmax; k++ {
				DT.DataP[k] = -100 // Global
			}
		}
		c.InterpolateSolutionToEdges(Q0, Q_Face) // Interpolates Q_Face values from Q
	case 1:
		rk.MaxWaveSpeed[myThread] =
			c.CalculateEdgeFlux(rk.Time, true, rk.Jdet, rk.DT, rk.Q_Face, SortedEdgeKeys, EdgeQ1, EdgeQ2) // Global
	case 2:
		c.Dissipation.CalculateEpsilonGradient(c, C0, myThread, Q0)
	case 3:
		c.StoreGradientEdgeFlux(SortedEdgeKeys, EdgeQ1)
	case 4:
		c.SetRTFluxInternal(Kmax, Jdet, Jinv, F_RT_DOF, Q0) // Updates F_RT_DOF with values from Q
		c.SetRTFluxOnEdges(myThread, Kmax, F_RT_DOF)
		if c.LocalTimeStepping {
			// Replicate local time step to the other solution points for each k
			for k := 0; k < Kmax; k++ {
				DT.DataP[k] = c.CFL / DT.DataP[k] // Set each element's DT to CFL/(max_wave_speed)
			}
			// Set the DT of all interior points of each element to the element DT
			for i := 1; i < c.dfr.SolutionElement.Np; i++ {
				for k := 0; k < Kmax; k++ {
					ind := k + Kmax*i
					DT.DataP[ind] = DT.DataP[k]
				}
			}
		}
		c.RHSInternalPoints(Kmax, Jdet, F_RT_DOF, RHSQ)
		c.Dissipation.AddDissipation(c, contLevel, myThread, Jinv, Jdet, Q0, RHSQ)
		dT = rk.GlobalDT
		for n := 0; n < 4; n++ {
			for i := 0; i < Kmax*Np; i++ {
				if c.LocalTimeStepping {
					dT = DT.DataP[i]
				}
				Q1[n].DataP[i] = Q0[n].DataP[i] + 0.5*RHSQ[n].DataP[i]*dT
			}
		}
		c.InterpolateSolutionToEdges(Q1, Q_Face) // Interpolates Q_Face values from Q
	case 5:
		c.CalculateEdgeFlux(rk.Time, false, rk.Jdet, rk.DT, rk.Q_Face, SortedEdgeKeys, EdgeQ1, EdgeQ2) // Global
	case 6:
		c.Dissipation.CalculateEpsilonGradient(c, C0, myThread, Q1)
	case 7:
		c.StoreGradientEdgeFlux(SortedEdgeKeys, EdgeQ1)
	case 8:
		c.SetRTFluxInternal(Kmax, Jdet, Jinv, F_RT_DOF, Q1) // Updates F_RT_DOF with values from Q
		c.SetRTFluxOnEdges(myThread, Kmax, F_RT_DOF)
		c.RHSInternalPoints(Kmax, Jdet, F_RT_DOF, RHSQ)
		c.Dissipation.AddDissipation(c, contLevel, myThread, Jinv, Jdet, Q1, RHSQ)
		dT = rk.GlobalDT
		for n := 0; n < 4; n++ {
			for i := 0; i < Kmax*Np; i++ {
				if c.LocalTimeStepping {
					dT = DT.DataP[i]
				}
				Q2[n].DataP[i] = Q1[n].DataP[i] + 0.25*RHSQ[n].DataP[i]*dT
			}
		}
		c.InterpolateSolutionToEdges(Q2, Q_Face) // Interpolates Q_Face values from Q
	case 9:
		c.CalculateEdgeFlux(rk.Time, false, rk.Jdet, rk.DT, rk.Q_Face, SortedEdgeKeys, EdgeQ1, EdgeQ2) // Global
	case 10:
		c.Dissipation.CalculateEpsilonGradient(c, C0, myThread, Q2)
	case 11:
		c.StoreGradientEdgeFlux(SortedEdgeKeys, EdgeQ1)
	case 12:
		c.SetRTFluxInternal(Kmax, Jdet, Jinv, F_RT_DOF, Q2) // Updates F_RT_DOF with values from Q
		c.SetRTFluxOnEdges(myThread, Kmax, F_RT_DOF)
		c.RHSInternalPoints(Kmax, Jdet, F_RT_DOF, RHSQ)
		c.Dissipation.AddDissipation(c, contLevel, myThread, Jinv, Jdet, Q2, RHSQ)
		dT = rk.GlobalDT
		for n := 0; n < 4; n++ {
			for i := 0; i < Kmax*Np; i++ {
				if c.LocalTimeStepping {
					dT = DT.DataP[i]
				}
				Q3[n].DataP[i] = (1. / 3.) * (2*Q0[n].DataP[i] + Q2[n].DataP[i] + RHSQ[n].DataP[i]*dT)
			}
		}
		c.InterpolateSolutionToEdges(Q3, Q_Face) // Interpolates Q_Face values from Q
	case 13:
		c.CalculateEdgeFlux(rk.Time, false, rk.Jdet, rk.DT, rk.Q_Face, SortedEdgeKeys, EdgeQ1, EdgeQ2) // Global
	case 14:
		c.Dissipation.CalculateEpsilonGradient(c, C0, myThread, Q3)
	case 15:
		c.StoreGradientEdgeFlux(SortedEdgeKeys, EdgeQ1)
	case 16:
		c.SetRTFluxInternal(Kmax, Jdet, Jinv, F_RT_DOF, Q3) // Updates F_RT_DOF with values from Q
		c.SetRTFluxOnEdges(myThread, Kmax, F_RT_DOF)
		c.RHSInternalPoints(Kmax, Jdet, F_RT_DOF, RHSQ)
		c.Dissipation.AddDissipation(c, contLevel, myThread, Jinv, Jdet, Q3, RHSQ)
		// Note, we are re-using Q1 as storage for Residual here
		dT = rk.GlobalDT
		for n := 0; n < 4; n++ {
			for i := 0; i < Kmax*Np; i++ {
				if c.LocalTimeStepping {
					dT = DT.DataP[i]
				}
				Residual[n].DataP[i] = Q3[n].DataP[i] + 0.25*RHSQ[n].DataP[i]*dT - Q0[n].DataP[i]
				Q0[n].DataP[i] += Residual[n].DataP[i]
			}
		}
		c.Limiter.LimitSolution(myThread, c.Q, rk.Residual)
	}
	return
}

func (c *Euler) RHSInternalPoints(Kmax int, Jdet utils.Matrix, F_RT_DOF, RHSQ [4]utils.Matrix) {
	var (
		JdetD = Jdet.DataP
		Nint  = c.dfr.FluxElement.Nint
		data  []float64
	)
	/*
				Calculate the RHS of the equation:
				dQ/dt = -div(F,G)
				Where:
					Q = [rho, rhoU, rhoV, E]
					F = [rhoU, rhoU*u+p, rhoV*u, u*(E+p)]
					G = [rhoV, rhoU*v, rhoV*v+p, v*(E+p)]

		    	The divergence div(F,G) is calculated using a Raviart Thomas finite element with flux (F,G) values on the faces
				of the element "injected" via calculation of a physical flux on those faces, and the (F,G) values in the interior
				of the element taken directly from the solution values (Q).
	*/
	for n := 0; n < 4; n++ { // For each of the 4 equations in [rho, rhoU, rhoV, E]
		// Unit triangle divergence matrix times the flux projected onto the RT elements: F_RT_DOF => Div(Flux) in (r,s)
		c.dfr.FluxElement.DivInt.Mul(F_RT_DOF[n], RHSQ[n])
		// Multiply each element's divergence by 1/||J|| to go (r,s)->(x,y), and -1 for the RHS
		data = RHSQ[n].DataP
		for k := 0; k < Kmax; k++ {
			var (
				oojd = 1. / JdetD[k]
			)
			for i := 0; i < Nint; i++ {
				ind := k + i*Kmax
				//data[ind] /= -JdetD[k]
				data[ind] *= -oojd
			}
		}
	}
}

func (c *Euler) SetRTFluxInternal(Kmax int, Jdet, Jinv utils.Matrix, F_RT_DOF, Q [4]utils.Matrix) {
	var (
		Nint   = c.dfr.FluxElement.Nint
		NpFlux = c.dfr.FluxElement.Np
		fdofD  = [4][]float64{F_RT_DOF[0].DataP, F_RT_DOF[1].DataP, F_RT_DOF[2].DataP, F_RT_DOF[3].DataP}
	)
	/*
		Zero out DOF storage to promote easier bug avoidance
	*/
	for n := 0; n < 4; n++ {
		for i := 0; i < Kmax*NpFlux; i++ {
			fdofD[n][i] = 0.
		}
	}
	// Calculate flux and project into R and S (transformed) directions for the internal points
	for k := 0; k < Kmax; k++ {
		for i := 0; i < Nint; i++ {
			ind := k + i*Kmax
			ind2 := k + (i+Nint)*Kmax
			Fr, Fs := c.CalculateFluxTransformed(k, Kmax, i, Jdet, Jinv, Q)
			for n := 0; n < 4; n++ {
				fdofD[n][ind], fdofD[n][ind2] = Fr[n], Fs[n]
			}
		}
	}
}

func (c *Euler) InitializeSolution(verbose bool) {
	// Initialize solution
	c.Q = make([][4]utils.Matrix, c.Partitions.ParallelDegree) // Allocate shards to store solution
	switch c.Case {
	case FREESTREAM:
		NP := c.Partitions.ParallelDegree
		for np := 0; np < NP; np++ {
			Kmax := c.Partitions.GetBucketDimension(np)
			c.Q[np] = c.InitializeFS(Kmax)
		}
	case SHOCKTUBE:
		c.SolutionX = c.ShardByK(c.dfr.SolutionX)
		c.SolutionY = c.ShardByK(c.dfr.SolutionY)
		c.FS.Gamma = 1.4
		GM1 := c.FS.Gamma - 1
		c.FS.Qinf = [4]float64{1, 0, 0, 1 / GM1}
		c.FS.Pinf = c.FS.GetFlowFunctionQQ(c.FS.Qinf, StaticPressure)
		c.FS.QQinf = c.FS.GetFlowFunctionQQ(c.FS.Qinf, DynamicPressure)
		c.FS.Cinf = c.FS.GetFlowFunctionQQ(c.FS.Qinf, SoundSpeed)
		NP := c.Partitions.ParallelDegree
		for np := 0; np < NP; np++ {
			var (
				Kmax = c.Partitions.GetBucketDimension(np)
				Nint = c.dfr.SolutionElement.Np
			)
			for n := 0; n < 4; n++ {
				c.Q[np][n] = utils.NewMatrix(Nint, Kmax)
			}
			for k := 0; k < Kmax; k++ {
				for i := 0; i < Nint; i++ {
					ind := k + i*Kmax
					if c.SolutionX[np].DataP[ind] > 0 {
						c.Q[np][0].DataP[ind] = c.FS.Qinf[0]
						c.Q[np][1].DataP[ind] = c.FS.Qinf[1]
						c.Q[np][2].DataP[ind] = c.FS.Qinf[2]
						c.Q[np][3].DataP[ind] = c.FS.Qinf[3]
					} else {
						c.Q[np][0].DataP[ind] = 0.125
						c.Q[np][1].DataP[ind] = 0
						c.Q[np][2].DataP[ind] = 0
						c.Q[np][3].DataP[ind] = 0.1 / GM1
					}
				}
			}
		}
	case IVORTEX:
		c.FS.Qinf = [4]float64{1, 1, 0, 3}
		c.FS.Pinf = c.FS.GetFlowFunctionQQ(c.FS.Qinf, StaticPressure)
		c.FS.QQinf = c.FS.GetFlowFunctionQQ(c.FS.Qinf, DynamicPressure)
		c.FS.Cinf = c.FS.GetFlowFunctionQQ(c.FS.Qinf, SoundSpeed)
		c.SolutionX = c.ShardByK(c.dfr.SolutionX)
		c.SolutionY = c.ShardByK(c.dfr.SolutionY)
		NP := c.Partitions.ParallelDegree
		for np := 0; np < NP; np++ {
			c.AnalyticSolution, c.Q[np] = c.InitializeIVortex(c.SolutionX[np], c.SolutionY[np])
		}
		// Set "Wall" BCs to IVortex
		var count int
		for _, e := range c.dfr.Tris.Edges {
			if e.BCType == types.BC_Wall {
				count++
				e.BCType = types.BC_IVortex
			}
		}
		if verbose {
			fmt.Printf("\tReplaced %d Wall boundary conditions with analytic BC_IVortex\n", count)
		}
	default:
		panic("unknown case type")
	}
}

func (c *Euler) CheckIfFinished(Time, FinalTime float64, steps int) (finished bool) {
	if Time >= FinalTime || steps >= c.MaxIterations {
		finished = true
	}
	return
}
func (c *Euler) PrintInitialization(FinalTime float64) {
	fmt.Printf("Using mesh from file: [%s]\n", c.MeshFile)
	if c.LocalTimeStepping {
		fmt.Printf("Solving until Max Iterations = %d\n", c.MaxIterations)
		fmt.Printf("    iter                ")
	} else {
		fmt.Printf("Solving until finaltime = %8.5f\n", FinalTime)
		fmt.Printf("    iter    time  min_dt")
	}
	fmt.Printf("       Res0       Res1       Res2")
	fmt.Printf("       Res3         L1         L2\n")
}
func (c *Euler) PrintUpdate(Time, dt float64, steps int, Q, Residual [][4]utils.Matrix, plotQ bool, pm *PlotMeta,
	printMem bool) {
	format := "%11.4e"
	if plotQ {
		c.PlotQ(pm) // wait till we implement time iterative frame updates
	}
	if c.LocalTimeStepping {
		fmt.Printf("%10d              ", steps)
	} else {
		fmt.Printf("%8d%8.5f%8.5f", steps, Time, dt)
	}
	var l1, l2 float64
	for n := 0; n < 4; n++ {
		var maxR float64
		for np := 0; np < c.Partitions.ParallelDegree; np++ {
			m := Residual[np][n].Max()
			if m > maxR {
				maxR = m
			}
		}
		fmt.Printf(format, maxR)
		if maxR > l1 {
			l1 = maxR
		}
		l2 += maxR * maxR
	}
	fmt.Printf(format, l1)
	fmt.Printf(format, math.Sqrt(l2)/4.)
	if printMem {
		fmt.Printf(" :: %s", utils.GetMemUsage())
	}
	fmt.Printf("\n")
}
func (c *Euler) PrintFinal(elapsed time.Duration, steps int) {
	rate := float64(elapsed.Microseconds()) / (float64(c.dfr.K * steps))
	fmt.Printf("\nRate of execution = %8.5f us/(element*iteration) over %d iterations\n", rate, steps)
	instructionRate := 1344.282 // measured instructions per element per iteration
	iRate := instructionRate * float64(c.dfr.K*steps) / elapsed.Seconds()
	var mem syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &mem)
	usec, ssec := float64(mem.Utime.Nano())/1.e9, float64(mem.Stime.Nano())/1.e9
	fmt.Printf("OS Stats: Utime=%6.2f(s), Stime=%6.2f(s), RSS Memory=%6.2f(MB)\n", usec, ssec, float64(mem.Maxrss)/1024.)
	fmt.Printf("Estimated op rate = %8.5f Giga-ops / second, based on measure op rate of %8.5f per element per iteration\n",
		iRate/1000000000., instructionRate)
}

func (rk *RungeKutta4SSP) calculateGlobalDT(c *Euler) {
	var (
		pm = c.Partitions
		NP = pm.ParallelDegree
	)
	// Must be done in controller/sync process
	var wsMaxAll float64
	for np := 0; np < NP; np++ {
		wsMaxAll = math.Max(wsMaxAll, rk.MaxWaveSpeed[np])
	}
	rk.GlobalDT = c.CFL / wsMaxAll
	if rk.Time+rk.GlobalDT > c.FinalTime {
		rk.GlobalDT = c.FinalTime - rk.Time
	}
}

func (c *Euler) GetSolutionGradientUsingRTElement(myThread, varNum int, Q [4]utils.Matrix, GradX, GradY, DOFX, DOFY utils.Matrix) {
	/*
		Dimensions:
			Q[4] should be Nint x Kmax
			All others should be NpFlux x Kmax
	*/
	var (
		dfr          = c.dfr
		NpInt        = dfr.FluxElement.Nint
		NpEdge       = dfr.FluxElement.Nedge
		KmaxGlobal   = c.Partitions.MaxIndex
		Kmax         = c.Partitions.GetBucketDimension(myThread)
		DXmd, DYmd   = dfr.DXMetric.DataP, dfr.DYMetric.DataP
		DOFXd, DOFYd = DOFX.DataP, DOFY.DataP
	)
	var Un float64
	for k := 0; k < Kmax; k++ {
		kGlobal := c.Partitions.GetGlobalK(k, myThread)
		for i := 0; i < NpInt; i++ {
			ind := k + i*Kmax
			indG := kGlobal + i*KmaxGlobal
			ind2 := k + (i+NpInt)*Kmax
			ind2G := kGlobal + (i+NpInt)*KmaxGlobal
			Un = Q[varNum].DataP[ind]
			DOFXd[ind], DOFYd[ind] = DXmd[indG]*Un, DYmd[indG]*Un
			DOFXd[ind2], DOFYd[ind2] = DXmd[ind2G]*Un, DYmd[ind2G]*Un
		}
	}
	// Load edge solution values from solution storage
	for k := 0; k < Kmax; k++ {
		kGlobal := c.Partitions.GetGlobalK(k, myThread)
		for edgeNum := 0; edgeNum < 3; edgeNum++ {
			edgeVals, sign := c.EdgeStore.GetEdgeValues(QFluxForGradient, myThread, k, varNum, edgeNum, dfr)
			var (
				ii    int
				shift = edgeNum * NpEdge
			)
			for i := 0; i < NpEdge; i++ {
				if sign < 0 {
					ii = NpEdge - 1 - i
				} else {
					ii = i
				}
				ind := k + (2*NpInt+i+shift)*Kmax
				indG := kGlobal + (2*NpInt+i+shift)*KmaxGlobal
				Un = edgeVals[ii]
				DOFXd[ind] = DXmd[indG] * Un
				DOFYd[ind] = DYmd[indG] * Un
			}
		}
	}
	// Calculate Grad(U)
	dfr.FluxElement.Div.Mul(DOFX, GradX) // X Derivative, Divergence x RT_DOF is X derivative for this DOF
	dfr.FluxElement.Div.Mul(DOFY, GradY) // Y Derivative, Divergence x RT_DOF is Y derivative for this DOF
}

func (c *Euler) GetSolutionGradient(myThread, varNum int, Q [4]utils.Matrix, GradX, GradY, DR, DS utils.Matrix) {
	/*
		Dimensions:
			Q[4] should be Nint x Kmax
			All others should be NpFlux x Kmax
	*/
	var (
		dfr    = c.dfr
		NpFlux = dfr.FluxElement.Np
		Kmax   = c.Partitions.GetBucketDimension(myThread)
	)
	dfr.FluxDr.Mul(Q[varNum], DR)
	dfr.FluxDs.Mul(Q[varNum], DS)
	for k := 0; k < Kmax; k++ {
		var (
			JinvD = dfr.Jinv.DataP[4*k : 4*(k+1)]
		)
		for i := 0; i < NpFlux; i++ {
			ind := k + i*Kmax
			GradX.DataP[ind] = JinvD[0]*DR.DataP[ind] + JinvD[2]*DS.DataP[ind]
			GradY.DataP[ind] = JinvD[1]*DR.DataP[ind] + JinvD[3]*DS.DataP[ind]
		}
	}
}
