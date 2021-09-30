package Euler2D

import (
	"fmt"
	"math"
	"sort"

	"github.com/notargets/gocfd/DG2D"

	"github.com/notargets/gocfd/utils"
)

type VertexToElement [][2]int32 // Vertex id is the first int32, element ID is the next

func (ve VertexToElement) Len() int           { return len(ve) }
func (ve VertexToElement) Swap(i, j int)      { ve[i], ve[j] = ve[j], ve[i] }
func (ve VertexToElement) Less(i, j int) bool { return ve[i][0] < ve[j][0] }
func (ve VertexToElement) Sort()              { sort.Sort(ve) }

func NewVertexToElement(EtoV utils.Matrix) (VtoE VertexToElement) {
	var (
		Kmax, Nverts = EtoV.Dims()
	)
	if Nverts != 3 {
		msg := fmt.Errorf("EtoV should have dimensions [Kmax,3] was [%d,%d]", Kmax, Nverts)
		panic(msg)
	}
	VtoE = make(VertexToElement, Kmax*3)
	var ii int
	for k := 0; k < Kmax; k++ {
		for i := 0; i < 3; i++ {
			VtoE[ii] = [2]int32{int32(EtoV.At(k, i)), int32(k)}
			ii++
		}
	}
	VtoE.Sort()
	return
}

func (ve VertexToElement) Shard(pm *PartitionMap) (veSharded []VertexToElement) {
	var (
		NPar             = pm.ParallelDegree
		lve              = len(ve)
		VertexPartitions = NewPartitionMap(NPar, lve) // This has to be re-done to honor vertex grouping
		ib               int
		vNum             int32
	)
	veSharded = make([]VertexToElement, NPar)
	approxBucketSize := VertexPartitions.GetBucketDimension(0)
	getShardedPair := func(vve [2]int32, pm *PartitionMap) (vves [2]int32) {
		nodeIDSharded, _, _ := pm.GetLocalK(int(vve[1]))
		vves = [2]int32{vve[0], int32(nodeIDSharded)}
		return
	}
	_ = getShardedPair
	for np := 0; np < NPar; np++ {
		for i := 0; i < approxBucketSize; i++ {
			//veSharded[np] = append(veSharded[np], getShardedPair(ve[ib], pm))
			veSharded[np] = append(veSharded[np], ve[ib])
			ib++
			if ib == lve {
				return
			}
		}
		vNum = ve[ib][0]
		for ib < lve && ve[ib][0] == vNum {
			//veSharded[np] = append(veSharded[np], getShardedPair(ve[ib], pm))
			veSharded[np] = append(veSharded[np], ve[ib])
			ib++
			if ib == lve {
				return
			}
		}
	}
	return
}

type ScalarDissipation struct {
	VtoE           []VertexToElement // Sharded vertex to element map, [2] is [vertID, ElementID_Sharded]
	Epsilon        []utils.Matrix    // Sharded Np x Kmax, Interpolated from element vertices
	EpsilonScalar  [][]float64       // Sharded scalar value of dissipation, one per element
	DissX, DissY   []utils.Matrix    // Sharded Np x Kmax, Dissipation Flux
	Diss2X, Diss2Y []utils.Matrix    // Sharded Np x Kmax, Second order X and Y derivative, add together for Divergence
	EpsVertex      []float64         // NVerts x 1, Aggregated (Max) of epsilon surrounding each vertex, Not sharded
	PMap           *PartitionMap     // Partition map for the solution shards in K
	U, UClipped    []utils.Matrix    // Sharded scratch areas for assembly and testing of solution values
	Clipper        utils.Matrix      // Matrix used to clip the topmost mode from the solution polynomial, used in shockfinder
	Element        *DG2D.LagrangeElement2D
	S0, Kappa      float64
}

func NewScalarDissipation(kappa float64, dfr *DG2D.DFR2D, pm *PartitionMap) (sd *ScalarDissipation) {
	var (
		el     = dfr.SolutionElement
		NPar   = pm.ParallelDegree
		Np     = el.Np
		order  = float64(el.N)
		NVerts = dfr.VX.Len()
		EToV   = dfr.Tris.EToV
	)
	sd = &ScalarDissipation{
		EpsVertex:     make([]float64, NVerts),
		EpsilonScalar: make([][]float64, NPar),    // Viscosity, constant over the element
		Epsilon:       make([]utils.Matrix, NPar), // Epsilon field, expressed over solution points
		DissX:         make([]utils.Matrix, NPar),
		DissY:         make([]utils.Matrix, NPar),
		Diss2X:        make([]utils.Matrix, NPar),
		Diss2Y:        make([]utils.Matrix, NPar),
		VtoE:          NewVertexToElement(EToV).Shard(pm),
		PMap:          pm,
		Element:       el,
		// Sharded working matrices
		U:        make([]utils.Matrix, NPar),
		UClipped: make([]utils.Matrix, NPar),
		S0:       1. / math.Pow(order, 4.),
		Kappa:    4.,
	}
	if kappa != 0. {
		sd.Kappa = kappa
	}
	for np := 0; np < NPar; np++ {
		sd.U[np] = utils.NewMatrix(Np, 1)
		sd.UClipped[np] = utils.NewMatrix(Np, 1)
		Kmax := pm.GetBucketDimension(np)
		sd.Epsilon[np] = utils.NewMatrix(Np, Kmax)
		sd.EpsilonScalar[np] = make([]float64, Kmax)
		sd.DissX[np] = utils.NewMatrix(Np, Kmax)
		sd.DissY[np] = utils.NewMatrix(Np, Kmax)
		sd.Diss2X[np] = utils.NewMatrix(Np, Kmax)
		sd.Diss2Y[np] = utils.NewMatrix(Np, Kmax)
	}
	/*
		The "Clipper" matrix drops the last mode from the polynomial and forms an alternative field of values at the node
		points based on a polynomial with one less term. In other words, if we have a polynomial of degree "p", expressed
		as values at Np node points, multiplying the Node point values vector by Clipper produces an alternative version
		of the node values based on truncating the last polynomial mode.
	*/
	{
		data := make([]float64, Np)
		for i := 0; i < Np; i++ {
			if i != Np-1 {
				data[i] = 1.
			} else {
				data[i] = 0.
			}
		}
		diag := utils.NewDiagMatrix(Np, data)
		sd.Clipper = sd.Element.V.Mul(diag).Mul(sd.Element.Vinv)
	}
	return
}

func (sd *ScalarDissipation) AddDissipationC0(calcViscoscity bool, myThread int, JdetAll []utils.Matrix, Qall, RHSQall [][4]utils.Matrix) {
	var (
		Jdet           = JdetAll[myThread]
		Q              = Qall[myThread]
		DissX, DissY   = sd.DissX[myThread], sd.DissY[myThread]
		Diss2X, Diss2Y = sd.Diss2X[myThread], sd.Diss2Y[myThread]
		RHSQ           = RHSQall[myThread]
		EpsilonScalar  = sd.EpsilonScalar[myThread]
		Np, KMax       = Q[0].Dims()
	)
	if calcViscoscity {
		sd.calculateElementViscosity(myThread, JdetAll, Qall)
	}
	for n := 0; n < 4; n++ {
		sd.Element.Dr.Mul(Q[n], DissX) // dQ/dR
		sd.Element.Ds.Mul(Q[n], DissY) // dQ/dS
		for k := 0; k < KMax; k++ {
			for i := 0; i < Np; i++ {
				ind := k + KMax*i
				DissX.DataP[ind] *= EpsilonScalar[k] // Scalar viscosity, constant within each k'th element
				DissY.DataP[ind] *= EpsilonScalar[k]
			}
		}
		sd.Element.Dr.Mul(DissX, Diss2X)
		sd.Element.Ds.Mul(DissY, Diss2Y)
		Diss2X.Add(Diss2Y) // Compose Divergence in R,S coordinates
		for k := 0; k < KMax; k++ {
			ooJdet2K := 1. / (Jdet.DataP[k] * Jdet.DataP[k]) // Second derivative requires divide by det^2
			for i := 0; i < Np; i++ {
				ind := k + KMax*i
				RHSQ[n].DataP[ind] -= ooJdet2K * Diss2X.DataP[ind]
			}
		}
	}
}

func (sd *ScalarDissipation) GetScalarEpsilonPlotField(c *Euler) (fld utils.Matrix) {
	for np := 0; np < sd.PMap.ParallelDegree; np++ {
		Np, KMax := sd.Epsilon[np].Dims()
		for k := 0; k < KMax; k++ {
			epsK := sd.EpsilonScalar[np][k]
			for i := 0; i < Np; i++ {
				ind := k + KMax*i
				sd.Epsilon[np].DataP[ind] = epsK
			}
		}
	}
	fld = c.RecombineShardsK(sd.Epsilon)
	return
}

func (sd *ScalarDissipation) calculateElementViscosity(myThread int, JdetAll []utils.Matrix, Qall [][4]utils.Matrix) {
	var (
		Rho      = Qall[myThread][0]
		Jdet     = JdetAll[myThread]
		Eps      = sd.EpsilonScalar[myThread]
		Kmax     = sd.PMap.GetBucketDimension(myThread)
		U        = sd.U[myThread]
		UClipped = sd.UClipped[myThread]
	)
	for k := 0; k < Kmax; k++ {
		var (
			eps0        = math.Sqrt(2.*Jdet.DataP[k]) / float64(sd.Element.N)
			Se          = math.Log10(sd.moment(k, Kmax, U, UClipped, Rho))
			left, right = sd.S0 - sd.Kappa, sd.S0 + sd.Kappa
			oo2kappa    = 0.5 / sd.Kappa
		)
		switch {
		case Se < left:
			Eps[k] = 0.
		case Se >= left && Se < right:
			Eps[k] = 0.5 * eps0 * (1. + math.Sin(math.Pi*oo2kappa*(Se-sd.S0)))
		case Se >= right:
			Eps[k] = eps0
		}
	}
}

func (sd *ScalarDissipation) moment(k, Kmax int, U, UClipped, Rho utils.Matrix) (m float64) {
	var (
		Np            = sd.Element.Np
		UD, UClippedD = U.DataP, UClipped.DataP
	)
	for i := 0; i < Np; i++ {
		ind := k + i*Kmax
		U.DataP[i] = Rho.DataP[ind]
	}
	/*
		Evaluate the L2 moment of (q - qalt) over the element, where qalt is the truncated version of q
		Here we don't bother using quadrature, we do a simple sum
	*/
	UClipped = sd.Clipper.Mul(U, UClipped)
	for i := 0; i < Np; i++ {
		t1 := UD[i] - UClippedD[i]
		m += t1 * t1 / (UD[i] * UD[i])
	}
	return
}
