package DG2D

import "C"
import (
	"fmt"
	"image/color"
	"math"
	"os"

	graphics2D "github.com/notargets/avs/geometry"

	"github.com/notargets/avs/chart2d"

	"github.com/notargets/gocfd/DG1D"

	"github.com/notargets/gocfd/utils"
)

type Elements2D struct {
	K, N, Nfp, Np, NFaces             int
	NODETOL                           float64
	R, S, VX, VY, VZ                  utils.Vector
	FMask, Fx, Fy                     utils.Matrix
	EToV, EToE, EToF                  utils.Matrix
	BCType                            utils.Matrix
	X, Y, Dr, Ds, FScale, LIFT        utils.Matrix
	Rx, Ry, Sx, Sy                    utils.Matrix
	xs, xr, ys, yr                    utils.Matrix
	V, Vinv, MassMatrix               utils.Matrix
	NX, NY                            utils.Matrix
	J, sJ                             utils.Matrix
	VmapM, VmapP, VmapB, VmapI, VmapO utils.Index
	MapB, MapI, MapO                  utils.Index
	Cub                               *Cubature
	FMaskI                            utils.Index
}

type Cubature struct {
	r, s, w                 utils.Vector
	W                       utils.Matrix
	V, Dr, Ds, VT, DrT, DsT utils.Matrix
	x, y, rx, sx, ry, sy, J utils.Matrix
	mm, mmCHOL              utils.Matrix
}

func NewElements2D(N int, meshFile string, plotMesh bool) (el *Elements2D) {
	// choose order to integrate exactly
	//CubatureOrder = int(math.Floor(2.0 * float64(N+1) * 3.0 / 2.0))
	//NGauss        = int(math.Floor(2.0 * float64(N+1)))
	//el.NewCube2D(CubatureOrder)
	el = &Elements2D{
		N:      N,
		Np:     (N + 1) * (N + 2) / 2,
		NFaces: 3,
	}
	el.ReadGambit2d(meshFile, plotMesh)
	//el.Startup2D()
	el.Startup2DDFR()
	//fmt.Println(el.X.Print("X"))
	//fmt.Println(el.Y.Print("Y"))

	var (
		xx = el.X.Transpose().Data()
		yy = el.Y.Transpose().Data()
	)
	s1 := make([][2]float64, len(xx))
	s2 := make([][2]float64, len(xx))
	s3 := make([][2]float64, len(xx))
	s := make([][2]float64, len(xx))
	for i := range xx {
		s1[i][0] = 0.25 * (xx[i] + 1)
		s1[i][1] = 0.25 * (yy[i] + 1)
		s2[i][0] = 0.25 * (xx[i] - 1)
		s2[i][1] = 0.25 * (yy[i] + 1)
		s3[i][0] = 0.25 * (xx[i] + 1)
		s3[i][1] = 0.25 * (yy[i] - 1)
		s[i][0] = s1[i][0] + s2[i][0] + s3[i][0]
		s[i][1] = s1[i][1] + s2[i][1] + s3[i][1]
	}
	var chart *chart2d.Chart2D
	if plotMesh {
		white := color.RGBA{
			R: 255,
			G: 255,
			B: 255,
			A: 0,
		}
		chart = PlotMesh(el.VX, el.VY, el.EToV, el.BCType, el.X, el.Y)
		ydata := el.Y.Transpose().Data()
		geom := make([]graphics2D.Point, len(ydata))
		for i, xval := range el.X.Transpose().Data() {
			geom[i].X[0] = float32(xval)
			geom[i].X[1] = float32(ydata[i])
		}
		_ = chart.AddVectors("basis", geom, s, chart2d.Solid, white)
		sleepForever()
	}

	/*
	  // build cubature node data for all elements
	  CubatureVolumeMesh2D(CubatureOrder);

	  // build Gauss node data for all element faces
	  GaussFaceMesh2D(NGauss);

	  Resize_cub();           // resize cubature arrays
	  MapGaussFaceData();     // {nx = gauss.nx}, etc.
	  PreCalcBdryData();      // gmapB = concat(mapI, mapO), etc.
	*/
	// N is the polynomial degree, Np is the number of interpolant points = N+1

	return
}

func (el *Elements2D) InterpMatrix2D() {
	/*
	   //---------------------------------------------------------
	   void NDG2D::InterpMatrix2D(Cub2D& cub)
	   //---------------------------------------------------------
	   {
	   // compute Vandermonde at (rout,sout)
	   DMat Vout = Vandermonde2D(this->N, cub.r, cub.s);
	   // build interpolation matrix
	   cub.V = Vout * this->invV;
	   // store transpose
	   cub.VT = trans(cub.V);
	   }
	   }
	*/
}

func Vandermonde2D(N int, r, s utils.Vector) (V2D utils.Matrix) {
	V2D = utils.NewMatrix(r.Len(), (N+1)*(N+2)/2)
	a, b := RStoAB(r, s)
	var sk int
	for i := 0; i <= N; i++ {
		for j := 0; j <= (N - i); j++ {
			V2D.SetCol(sk, Simplex2DP(a, b, i, j))
			sk++
		}
	}
	return
}

func Simplex2DP(a, b utils.Vector, i, j int) (P []float64) {
	var (
		Np = a.Len()
		bd = b.Data()
	)
	h1 := DG1D.JacobiP(a, 0, 0, i)
	h2 := DG1D.JacobiP(b, float64(2*i+1), 0, j)
	P = make([]float64, Np)
	sq2 := math.Sqrt(2)
	for ii := range h1 {
		tv1 := sq2 * h1[ii] * h2[ii]
		tv2 := utils.POW(1-bd[ii], i)
		P[ii] = tv1 * tv2
	}
	return
}

/*
	  // evaluate generalized Vandermonde of Lagrange interpolant functions at cubature nodes
	  InterpMatrix2D(m_cub);

	  // evaluate local derivatives of Lagrange interpolants at cubature nodes
	  Dmatrices2D(this->N, m_cub);

	  // evaluate the geometric factors at the cubature nodes
	  GeometricFactors2D(m_cub);

	  // custom mass matrix per element
	  DMat mmk; DMat_Diag D; DVec d;
	  m_cub.mmCHOL.resize(Np*Np, K);
	  m_cub.mm    .resize(Np*Np, K);

	  for (int k=1; k<=K; ++k) {
	    d=m_cub.J(All,k); d*=m_cub.w; D.diag(d);  // weighted diagonal
	    mmk = m_cub.VT * D * m_cub.V;     // mass matrix for element k
	    m_cub.mm(All,k)     = mmk;        // store mass matrix
	    m_cub.mmCHOL(All,k) = chol(mmk);  // store Cholesky factorization
	  }

	  // incorporate weights and Jacobian
	  m_cub.W = outer(m_cub.w, ones(K));
	  m_cub.W.mult_element(m_cub.J);

	  // compute coordinates of cubature nodes
	  m_cub.x = m_cub.V * this->x;
	  m_cub.y = m_cub.V * this->y;

	  return m_cub;
	}
*/

func NodesEpsilon(N int) (R, S utils.Vector) {
	/*
		From the 2017 paper "A Direct Flux Reconstruction Scheme for Advection Diffusion Problems on Triangular Grids"

		This is a node set that is compatible with DFR in that it implements colocated solution and flux points for the
		interior nodes, while enabling a set of face nodes for the N+1 degree flux polynomial

		There are two node sets, one for N=3 and one for N=4. They were computed via an optimization, and are only
		available for N=3 and N=4. Also, the convergence of N=3 is degraded for diffusion problems.

		Therefore, only the N=4 points should be used for Viscous solutions, while the N=3 nodes are fine for inviscid
	*/
	var (
		Np   = (N + 1) * (N + 2) / 2
		epsD []float64
	)
	switch N {
	case 0:
		R = utils.NewVector(1, []float64{-.5})
		S = utils.NewVector(1, []float64{-.5})
		return
	case 3:
		epsD = []float64{
			0.3333333333333333, 0.055758983558155, 0.88848203288369, 0.055758983558155, 0.290285227512689, 0.6388573870878149, 0.290285227512689, 0.6388573870878149, 0.070857385399496, 0.070857385399496,
			0.3333333333333333, 0.055758983558155, 0.055758983558155, 0.88848203288369, 0.070857385399496, 0.290285227512689, 0.6388573870878149, 0.070857385399496, 0.290285227512689, 0.6388573870878149,
			0.3333333333333333, 0.88848203288369, 0.055758983558155, 0.055758983558155, 0.6388573870878149, 0.070857385399496, 0.070857385399496, 0.290285227512689, 0.6388573870878149, 0.290285227512689,
		}
	case 4:
		epsD = []float64{
			0.034681580220044, 0.9306368395599121, 0.034681580220044, 0.243071555674492, 0.513856888651016, 0.243071555674492, 0.473372556704605, 0.05325488659079003, 0.473372556704605, 0.200039998995093, 0.752666332493468, 0.200039998995093, 0.752666332493468, 0.047293668511439, 0.047293668511439,
			0.034681580220044, 0.034681580220044, 0.9306368395599121, 0.243071555674492, 0.243071555674492, 0.513856888651016, 0.473372556704605, 0.473372556704605, 0.05325488659079003, 0.047293668511439, 0.200039998995093, 0.752666332493468, 0.047293668511439, 0.200039998995093, 0.752666332493468,
			0.9306368395599121, 0.034681580220044, 0.034681580220044, 0.513856888651016, 0.243071555674492, 0.243071555674492, 0.05325488659079003, 0.473372556704605, 0.473372556704605, 0.752666332493468, 0.047293668511439, 0.047293668511439, 0.200039998995093, 0.752666332493468, 0.200039998995093,
		}
	default:
		panic(fmt.Errorf("Epsilon nodes not defined for N = %v, only defined for N=3 or N=4\n", N))
	}
	eps := utils.NewMatrix(3, Np, epsD)
	T := utils.NewMatrix(2, 3, []float64{
		-1, 1, -1,
		-1, -1, 1,
	})
	RS := T.Mul(eps)
	R = RS.Row(0)
	S = RS.Row(1)
	return
}

// Purpose  : Compute (x,y) nodes in equilateral triangle for
//            polynomial of order N
func Nodes2D(N int) (x, y utils.Vector) {
	var (
		alpha                                                               float64
		Np                                                                  = (N + 1) * (N + 2) / 2
		L1, L2, L3                                                          utils.Vector
		blend1, blend2, blend3, warp1, warp2, warp3, warpf1, warpf2, warpf3 []float64
	)
	L1, L2, L3, x, y =
		utils.NewVector(Np), utils.NewVector(Np), utils.NewVector(Np), utils.NewVector(Np), utils.NewVector(Np)
	l1d, l2d, l3d, xd, yd := L1.Data(), L2.Data(), L3.Data(), x.Data(), y.Data()
	blend1, blend2, blend3, warp1, warp2, warp3 =
		make([]float64, Np), make([]float64, Np), make([]float64, Np), make([]float64, Np), make([]float64, Np), make([]float64, Np)

	alpopt := []float64{
		0.0000, 0.0000, 1.4152, 0.1001, 0.2751,
		0.9800, 1.0999, 1.2832, 1.3648, 1.4773,
		1.4959, 1.5743, 1.5770, 1.6223, 1.6258,
	}
	if N < 16 {
		alpha = alpopt[N-1]
	} else {
		alpha = 5. / 3.
	}
	// Create equidistributed nodes on equilateral triangle
	fn := 1. / float64(N)
	var sk int
	for n := 0; n < N+1; n++ {
		for m := 0; m < (N + 1 - n); m++ {
			l1d[sk] = float64(n) * fn
			l3d[sk] = float64(m) * fn
			sk++
		}
	}
	for i := range xd {
		l2d[i] = 1 - l1d[i] - l3d[i]
		xd[i] = l3d[i] - l2d[i]
		yd[i] = (2*l1d[i] - l3d[i] - l2d[i]) / math.Sqrt(3)
		// Compute blending function at each node for each edge
		blend1[i] = 4 * l2d[i] * l3d[i]
		blend2[i] = 4 * l1d[i] * l3d[i]
		blend3[i] = 4 * l1d[i] * l2d[i]
	}
	// Amount of warp for each node, for each edge
	warpf1 = Warpfactor(N, L3.Copy().Subtract(L2))
	warpf2 = Warpfactor(N, L1.Copy().Subtract(L3))
	warpf3 = Warpfactor(N, L2.Copy().Subtract(L1))
	// Combine blend & warp
	for i := range warpf1 {
		warp1[i] = blend1[i] * warpf1[i] * (1 + utils.POW(alpha*l1d[i], 2))
		warp2[i] = blend2[i] * warpf2[i] * (1 + utils.POW(alpha*l2d[i], 2))
		warp3[i] = blend3[i] * warpf3[i] * (1 + utils.POW(alpha*l3d[i], 2))
	}
	// Accumulate deformations associated with each edge
	for i := range xd {
		xd[i] += warp1[i] + math.Cos(2*math.Pi/3)*warp2[i] + math.Cos(4*math.Pi/3)*warp3[i]
		yd[i] += math.Sin(2*math.Pi/3)*warp2[i] + math.Sin(4*math.Pi/3)*warp3[i]
	}
	return
}

func Warpfactor(N int, rout utils.Vector) (warpF []float64) {
	var (
		Nr   = rout.Len()
		Pmat = utils.NewMatrix(N+1, Nr)
	)
	// Compute LGL and equidistant node distribution
	LGLr := DG1D.JacobiGL(0, 0, N)
	req := utils.NewVector(N+1).Linspace(-1, 1)
	Veq := DG1D.Vandermonde1D(N, req)
	// Evaluate Lagrange polynomial at rout
	for i := 0; i < (N + 1); i++ {
		Pmat.M.SetRow(i, DG1D.JacobiP(rout, 0, 0, i))
	}
	Lmat := Veq.Transpose().LUSolve(Pmat)
	// Compute warp factor
	warp := Lmat.Transpose().Mul(LGLr.Subtract(req).ToMatrix())
	// Scale factor
	zerof := rout.Copy().Apply(func(val float64) (res float64) {
		if math.Abs(val) < (1.0 - (1e-10)) {
			res = 1.
		}
		return
	})
	sf := zerof.Copy().ElMul(rout).Apply(func(val float64) (res float64) {
		res = 1 - val*val
		return
	})
	w2 := warp.Copy()
	warp.ElDiv(sf.ToMatrix()).Add(w2.ElMul(zerof.AddScalar(-1).ToMatrix()))
	warpF = warp.Data()
	return
}

func (el *Elements2D) Startup2DDFR() {
	var (
	//err error
	)
	el.Nfp = el.N + 1
	el.Np = (el.N + 1) * (el.N + 2) / 2
	el.NFaces = 3
	el.NODETOL = 1.e-12
	// Build reference element matrices
	/*
			We build the mixed elements for the DFR scheme with:

			Solution Points: We use points within a reference triangle, excluding the edges, for a Lagrangian element
			of O(K) to store the solution. If we need derivatives, or interpolated quantities (Flux), we use the
			solution points element.

			Flux Points: We use a customized Raviart-Thomas (RT) vector element of O(K+1) to store the vector Flux function
		    computed from the solution values. The RT element is of order O(K+1) and is a combination of the points from
			the solution element for the interior, and points along the three triangle edges. The custom RT basis is
			established using a procedure outlined in: "A Direct Flux Reconstruction Scheme for Advection-Diffusion
			Problems on Triangular Grids" by Romero, Witherden and Jameson (2017). A complete RT basis, [ B ], is used
			together with unit basis vectors, [ w ], to satisfy the following:
					[ B_j(r_i) dot w_i ] [ C ] = [ delta_i_j ]
					=> solve for [ C ], the coefficients defining the custom RT basis

			[ C ] is the vector of coefficients defining the basis using the basis vectors [ w ] and [ B ].

			The [ w ] directions of the custom RT element basis are defined such that:
				w([r]) = w(edge_locations) = unit normals on each of three edges
				w([r]) = w(interior) = unit normals in the two primary geometry directions (r and s)

			For order K there are:
				- (K+1) locations on each edge, for a total of 3(K+1) edge basis functions.
				- (K)(K+1) locations in the interior, half for the w_r direction and half for the w_s direction
				- Total: (K+3)(K+1) basis functions for the custom RT_K element

			Notes:
				1) The number of interior points matches the Lagrangian element in 2D at order (K-1). A Lagrange element
				at order (K) has N_p = (K+1)(K+2)/2 degrees of freedom, so an order (K-1) element has (K)(K+1)/2 DOF.
				Considering that we need a term for each of the two interior directions at each interior point, we need
				exactly 2*N_p DOF at order (K-1) for the interior of the custom RT element, resulting in (K)(K+1) terms.
				2) Note (1) confirms that the custom element requires exactly the same number of interior points
				(K)(K+1)/2 as a Lagrange element of order (K-1), which means we can use the custom RT element for the
				DFR approach, which needs to provide a O(K+1) element to preserve the gradient at O(K). We will use the
				solution points from the Lagrange element at O(K) to construct the interior of the O(K+1) RT element
				without requiring interpolation of the solution points, as they already reside at the same geometric
				locations.
				(3) To create the custom RT element, we initialize the solution element, then define the custom RT element
				from the interior point locations of the solution element to ensure that they are colocated.
				(4) To use the custom RT element:
				a) calculate the solution, calculate the flux vector field from the solution at the solution points
				b) transfer the flux vector field values to the DFR element interior
				c) interpolate flux values at from the interior of the RT element to the locations on the triangle edges
				d) use the method of characteristics to calculate the corrected flux using the neighbor element's edge
				flux combined with the edge flux from this element
				e) calculate the gradient of the vector flux field using the custom RT element
				f) transfer the gradient values from the RT element to the solution element for use in advancing the
				solution in differential form (directly)

			By calculating the flux gradient in a way that yields an O(K) polynomial on the solution points, we can use
			the differential form of the equations directly for the solution, rather than using the traditional Galerkin
			approach of repeated integration by parts to obtain an equation with only first derivatives. This simplifies
			the solution process, resulting in a more efficient computational approach, in addition to making it easier
			to solve more complex equations with the identical formulation.
	*/
	//el.V = Vandermonde2D(el.N, el.R, el.S) // Lagrange Element for solution points (not used)
	// Compute nodal set
	fmt.Printf("N input = %d\n", el.N)
	el.R, el.S = NodesEpsilon(el.N)
	//el.RTCustom()
	RTBasis(el.N+1, el.R, el.S)
}

func (el *Elements2D) RTCustom() (RT1, RT2 utils.Matrix) {
	var (
		N          = el.N + 1
		NpInternal = N * (N + 1) / 2
		NpEdge     = N + 1
		Np         = 3*NpEdge + 2*NpInternal
	)
	RT1, RT2 = el.RaviartThomasSimplex(N, el.R, el.S)
	// Customize RT basis using separate basis vectors for each internal location and one for each edge normal
	oosr2 := 1. / math.Sqrt(2)
	e1 := []float64{oosr2, oosr2}
	e2 := []float64{-1, 0}
	e3 := []float64{0, -1}
	e4 := []float64{1, 0}
	e5 := []float64{0, 1}
	// Construct right vectors for each point within the basis, one for each direction r and s
	er, es := make([]float64, Np), make([]float64, Np)
	for i := 0; i < NpEdge; i++ {
		er[i] = e1[0]
		er[i+NpEdge] = e2[0]
		er[i+2*NpEdge] = e3[0]
		es[i] = e1[1]
		es[i+NpEdge] = e2[1]
		es[i+2*NpEdge] = e3[1]
	}
	for i := 0; i < NpInternal; i++ {
		er[i+3*NpEdge] = e4[0]
		er[i+NpInternal+3*NpEdge] = e5[0]
		es[i+3*NpEdge] = e4[1]
		es[i+NpInternal+3*NpEdge] = e5[1]
	}
	// Construct the dot product of the new basis with the RT basis
	A := utils.NewMatrix(Np, Np)
	rowProduct := make([]float64, Np)
	for irow := 0; irow < Np; irow++ {
		row1 := RT1.Row(irow).Data()
		row2 := RT2.Row(irow).Data()
		for j := 0; j < Np; j++ {
			rowProduct[j] = row1[j]*er[j] + row2[j]*es[j]
		}
		A.M.SetRow(irow, rowProduct)
	}
	fmt.Println(A.Print("A"))
	S := utils.NewMatrix(Np, 1).AddScalar(1)
	var Ainv utils.Matrix
	var err error
	if Ainv, err = A.Inverse(); err != nil {
		panic(err)
	}
	os.Exit(1)
	c := Ainv.Mul(S)
	fmt.Println(c.Print("c"))
	fmt.Println(RT1.Print("RT1"))
	fmt.Println(RT2.Print("RT2"))
	return
}

func RTBasis(N int, R, S utils.Vector) {
	/*
					This is constructed from the defining space of the RT element:
									 2
						RT_k = [(P_k) ]   + [ X ] P_k
							 = [ b1(r,s)_i + r * b3(r,s)_j ]
			    			   [ b2(r,s)_i + s * b3(r,s)_j ]
		   				i := 1, (K+1)(K+2)/2
		   				j := (K+1)(K+2)/2 - K, (K+1)(K+2)/2 (highest order terms in polynomial)
					The dimension of RT_k is (K+1)(K+3) and we can see from the above that the total
					number of terms in the polynomial will be:
						2*(K+1)(K+2)/2 + K+1
						= (K+1)(K+3)

					The explanation for why the b3 polynomial sub-basis is partially consumed:
					When multiplied by [ X ], the b3 polynomial produces terms redundant with
					the b1 and b2 sub-bases. The redundancy is removed from the b3 sub-basis to
					compensate, producing the correct overall dimension.

					This routine will evaluate the polynomial for order N at all points provided
					in the r and s []float64 and return the resulting vector [p1,p2] for each input.
	*/
	var (
		Np        = (N + 1) * (N + 3)
		p1, p2    = make([]float64, Np), make([]float64, Np)
		Nsub1     = (N + 1) * (N + 2) / 2
		NInterior = N * (N + 1) / 2 // one order less than RT element in (P_k)2
	)
	/*
		Transform the incoming (r,s) locations into the unit triangle (0,1) from the (-1,1) basis
	*/
	R.AddScalar(1).Scale(0.5)
	S.AddScalar(1).Scale(0.5)
	// Add the edge points from the RT basis
	R, S = AddEdgePoints(N, R, S)
	fmt.Println(R.Transpose().Print("R"))
	fmt.Println(S.Transpose().Print("S"))
	/*
		First, evaluate the polynomial at the (r,s) coordinates
		This is the same set that will be used for all dot products to form the basis matrix
	*/
	Term2D := func(r, s float64, i, j int) (val float64) {
		// Note this only outputs the value of the (i,j)th term of the 2D polynomial
		val = utils.POW(r, j) * utils.POW(s, i)
		return
	}

	/*
		Form the basis matrix by forming a dot product with unit vectors, matching the coordinate locations in R,S
		The R,S set should be ordered such that the (N+1)(N+2)/2 interior points are first, followed by the
		3(N+1) triangle edge locations
	*/
	A := utils.NewMatrix(Np, Np)
	rowEdge := make([]float64, Np)
	oosr2 := 1 / math.Sqrt(2)

	// Evaluate at interior geometric locations
	for ii, rr := range R.Data() {
		ss := S.Data()[ii]
		// Evaluate the full 2D polynomial basis first, once for each of two components
		var sk int
		for i := 0; i <= N; i++ {
			for j := 0; j <= (N - i); j++ {
				val := Term2D(rr, ss, i, j)
				p1[sk] = val
				p2[sk+Nsub1] = val
				sk++
			}
		}
		fmt.Printf("p1, p2 = %v, %v\n", p1, p2)
		// Evaluate the term ([ X ]*(Pk)) at only the top N+1 terms (highest order) of the 2D polynomial
		for i := 0; i <= N; i++ {
			j := N - i
			val := Term2D(rr, ss, i, j)
			p1[sk+Nsub1] = val * rr
			p2[sk+Nsub1] = val * ss
			sk++
		}
		switch {
		case ii < NInterior:
			// Unit vector is [1,0]
			A.M.SetRow(ii, p1)
		case ii >= NInterior && ii < 2*NInterior:
			// Unit vector is [0,1]
			A.M.SetRow(ii, p2)
		case ii >= 2*NInterior && ii < 2*NInterior+(N+1):
			// Triangle Edge1
			for i := range rowEdge {
				// Edge1: Unit vector is [1/sqrt(2), 1/sqrt(2)]
				rowEdge[i] = oosr2 * (p1[i] + p2[i])
			}
			A.M.SetRow(ii, rowEdge)
		case ii >= 2*NInterior+(N+1) && ii < 2*NInterior+2*(N+1):
			for i := range rowEdge {
				// Edge2: Unit vector is [-1,0]
				rowEdge[i] = -p1[i]
			}
			A.M.SetRow(ii, rowEdge)
		case ii >= 2*NInterior+2*(N+1) && ii < 2*NInterior+3*(N+1):
			for i := range rowEdge {
				// Edge3: // Unit vector is [0,-1]
				rowEdge[i] = -p2[i]
			}
			A.M.SetRow(ii, rowEdge)
		}
	}
	fmt.Println(A.Print("A"))
	var Ainv utils.Matrix
	var err error
	if Ainv, err = A.Inverse(); err != nil {
		panic(err)
	}
	fmt.Println(Ainv.Print("Ainv"))
	os.Exit(1)
	return
}

func AddEdgePoints(N int, rInt, sInt utils.Vector) (r, s utils.Vector) {
	var (
		NpEdge       = N + 1
		rData, sData = rInt.Data(), sInt.Data()
	)
	/*
		Determine geometric locations of edge points, located at Gauss locations in 1D, projected onto the edges
	*/
	GQR, _ := DG1D.JacobiGQ(1, 1, N)
	// Transform into the space (0,1) from (-1,1)
	GQR.AddScalar(1).Scale(0.5)

	/*
		Double the number of interior points to match each direction of the basis
	*/
	rData = append(rData, rData...)
	sData = append(sData, sData...)

	// Calculate the triangle edges
	GQRData := GQR.Data()
	rEdgeData := make([]float64, NpEdge*3)
	sEdgeData := make([]float64, NpEdge*3)
	for i := 0; i < NpEdge; i++ {
		// Edge 1 (hypotenuse)
		rEdgeData[i] = 1 - GQRData[i]
		sEdgeData[i] = GQRData[i]
		// Edge 2
		rEdgeData[i+NpEdge] = 0
		sEdgeData[i+NpEdge] = 1 - GQRData[i]
		// Edge 3
		rEdgeData[i+2*NpEdge] = GQRData[i]
		sEdgeData[i+2*NpEdge] = 0
	}
	rData = append(rData, rEdgeData...)
	sData = append(sData, sEdgeData...)
	r = utils.NewVector(len(rData), rData)
	s = utils.NewVector(len(sData), sData)
	return
}

func (el *Elements2D) RaviartThomasSimplex(N int, r, s utils.Vector) (RT1, RT2 utils.Matrix) {
	/*
		Basis definition taken from "Computational Bases for RTk and BDMk on Triangles", V.J. Ervin, 2012
	*/
	var (
		NpInternal   = N * (N + 1) / 2
		NpEdge       = N + 1
		Np           = 3*NpEdge + 2*NpInternal
		rData, sData = r.Data(), s.Data()
	)
	if r.Len() != s.Len() {
		panic("number of internal points in each direction must be equal")
	}
	/*
		Note: This basis is possibly degenerate in that the same geometric points are used twice within the basis
		It may be that the basis is still orthogonal in that each term at the interior point can consume one of the
		two vector directions at each geometric point of evaluation - this is enforced for the custom RT basis.
	*/
	if r.Len() != NpInternal && r.Len() != 2*NpInternal {
		panic(
			fmt.Errorf("number of internal element locations must equal either %d, or %d, have %d",
				NpInternal, 2*NpInternal, r.Len()),
		)
	}
	fmt.Printf("Order = %d, Internal Order = %d, Internal point count = %d\n", N, N-1, NpInternal)
	// Allocate space for each vector component of the basis, 1 and 2 for the r and s directions
	RT1 = utils.NewMatrix(Np, Np)
	RT2 = utils.NewMatrix(Np, Np)

	/*
		If the number of supplied interior points is NpInternal, we use the internal geometric points twice, once for
		each class of interior basis. The degenerate basis will be corrected in the custom element by fitting each class
		to a unit vector orthogonal to the other class.
		If the number of points is 2*NpInternal, we use the provided points unaltered to produce a complete RT basis.
	*/
	if r.Len() == NpInternal {
		// Element will be used to produce a custom RT basis, correcting the degenerate basis later
		rData = append(rData, rData...)
		sData = append(sData, sData...)
		r = utils.NewVector(len(rData), rData)
		s = utils.NewVector(len(sData), sData)
	}

	/*
		Determine geometric locations of edge points, located at Gauss locations in 1D, projected onto the edges
	*/
	GQR, _ := DG1D.JacobiGQ(1, 1, N)
	GQR.AddScalar(1).Scale(0.5)

	GQRData := GQR.Data()
	rEdgeData := make([]float64, NpEdge*3)
	sEdgeData := make([]float64, NpEdge*3)
	for i := 0; i < NpEdge; i++ {
		// Edge 1 (hypotenuse)
		rEdgeData[i] = 2*(-GQRData[i]+1) - 1
		sEdgeData[i] = 2*GQRData[i] - 1
		// Edge 2
		rEdgeData[i+NpEdge] = -1
		sEdgeData[i+NpEdge] = 2*(1-GQRData[i]) - 1
		// Edge 3
		rEdgeData[i+2*NpEdge] = 2*GQRData[i] - 1
		sEdgeData[i+2*NpEdge] = -1
	}
	rEdge := utils.NewVector(NpEdge*3, rEdgeData)
	sEdge := utils.NewVector(NpEdge*3, sEdgeData)
	_, _ = rEdge, sEdge

	rData = append(rData, rEdgeData...)
	sData = append(sData, sEdgeData...)
	r = utils.NewVector(len(rData), rData)
	s = utils.NewVector(len(sData), sData)

	/*
		Convert the geometric points at (r,s) to the (a,b) coordinates needed for the Simplex2DP function
	*/
	a, b := RStoAB(r, s)
	/*
		Internal basis evaluated at internal points
		psi4(r, s) = P(r,s)*[s*r, s*(s-1)]
		psi5(r, s) = P(r,s)*[r*(r-1), r*s]
	*/
	var column int
	// Psi1 through Psi3, corresponding to edges 1-3
	// psi1(r,s) = P(s) * sqrt(2) * [ r, s ]
	sr2 := math.Sqrt(2)
	for i := 0; i < NpEdge; i++ {
		p := DG1D.JacobiP(s, 0, 0, N)
		p2 := append([]float64{}, p...)
		for ii := range p {
			p[ii] *= sr2 * rData[i]
			p2[ii] *= sr2 * sData[i]
		}
		RT1.SetCol(column, p)
		RT2.SetCol(column, p2)
		column++
	}
	// psi2(r,s) = P_(alpha=N+2-j)_(s) * [ r-1, s ]
	for i := 0; i < NpEdge; i++ {
		p := DG1D.JacobiP(s, float64(N+1-i), 0, N)
		p2 := append([]float64{}, p...)
		for ii := range p {
			p[ii] *= rData[i] - 1
			p2[ii] *= sData[i]
		}
		RT1.SetCol(column, p)
		RT2.SetCol(column, p2)
		column++
	}
	// psi3(r,s) = P(r) * [ r, s-1 ]
	for i := 0; i < NpEdge; i++ {
		p := DG1D.JacobiP(r, 0, 0, N)
		p2 := append([]float64{}, p...)
		for ii := range p {
			p[ii] *= rData[i]
			p2[ii] *= sData[i] - 1
		}
		RT1.SetCol(column, p)
		RT2.SetCol(column, p2)
		column++
	}

	// Psi4
	// psi4(r, s) = P(r,s)*[s*r, s*(s-1)]
	for i := 0; i <= N-1; i++ {
		for j := 0; j <= (N - 1 - i); j++ {
			// Evaluate at interior geometric locations
			p := Simplex2DP(a, b, i, j)
			p2 := append([]float64{}, p...)
			for ii := range p {
				p[ii] *= sData[ii] * rData[ii]
				p2[ii] *= sData[ii] * (sData[ii] - 1)
			}
			RT1.SetCol(column, p)
			RT2.SetCol(column, p2)
			column++
		}
	}
	// Psi5
	// psi5(r, s) = P(r,s)*[r*(r-1), r*s]
	for i := 0; i <= N-1; i++ {
		for j := 0; j <= (N - 1 - i); j++ {
			p := Simplex2DP(a, b, i, j)
			p2 := append([]float64{}, p...)
			for ii := range p {
				p[ii] *= rData[ii] * (rData[ii] - 1)
				p2[ii] *= rData[ii] * sData[ii]
			}
			RT1.SetCol(column, p)
			RT2.SetCol(column, p2)
			column++
		}
	}
	/*
		Project into the -1,1 reference triangle space using the Piola transformation, multiply by (J / det(J))
		where J is the jacobian of the transform from the unit triangle to the -1,1 reference triangle
		J = | 2   0 |
			| 0   2 |
		det(J) = 4
		So we need to multiply the above matrix by:
		J / det(J) = | 0.5  0   |
					 | 0    0.5 |
		Note that we need to use the same transform to map operations using this matrix from the -1,1 reference triangle
		to the real space in (x,y).
	*/
	RT1.Scale(0.5)
	RT2.Scale(0.5)
	return
}

func (el *Elements2D) Startup2D() {
	var (
		err error
	)
	el.Nfp = el.N + 1
	el.Np = (el.N + 1) * (el.N + 2) / 2
	el.NFaces = 3
	el.NODETOL = 1.e-12
	// Compute nodal set
	//el.R, el.S = NodesEpsilon(el.N)
	el.R, el.S = XYtoRS(Nodes2D(el.N))
	// Build reference element matrices
	el.V = Vandermonde2D(el.N, el.R, el.S)
	if el.Vinv, err = el.V.Inverse(); err != nil {
		panic(err)
	}
	el.MassMatrix = el.Vinv.Transpose().Mul(el.Vinv)
	// Initialize the (r,s) differentiation matrices on the simplex, evaluated at (r,s) at order N
	Vr, Vs := GradVandermonde2D(el.N, el.R, el.S)
	el.Dr = Vr.Mul(el.Vinv)
	el.Ds = Vs.Mul(el.Vinv)

	// build coordinates of all the nodes
	va, vb, vc := el.EToV.Col(0), el.EToV.Col(1), el.EToV.Col(2)
	el.X = el.R.Copy().Add(el.S).Scale(-1).Outer(el.VX.SubsetIndex(va.ToIndex())).Add(
		el.R.Copy().AddScalar(1).Outer(el.VX.SubsetIndex(vb.ToIndex()))).Add(
		el.S.Copy().AddScalar(1).Outer(el.VX.SubsetIndex(vc.ToIndex()))).Scale(0.5)
	el.Y = el.R.Copy().Add(el.S).Scale(-1).Outer(el.VY.SubsetIndex(va.ToIndex())).Add(
		el.R.Copy().AddScalar(1).Outer(el.VY.SubsetIndex(vb.ToIndex()))).Add(
		el.S.Copy().AddScalar(1).Outer(el.VY.SubsetIndex(vc.ToIndex()))).Scale(0.5)
	fmask1 := el.S.Copy().AddScalar(1).Find(utils.Less, el.NODETOL, true)
	fmask2 := el.S.Copy().Add(el.R).Find(utils.Less, el.NODETOL, true)
	fmask3 := el.R.Copy().AddScalar(1).Find(utils.Less, el.NODETOL, true)
	if fmask1.Len() != 0 {

		el.FMask = utils.NewMatrix(el.Nfp, 3)
		el.FMask.SetCol(0, fmask1.Data())
		el.FMask.SetCol(1, fmask2.Data())
		el.FMask.SetCol(2, fmask3.Data())
		el.FMaskI = utils.NewIndex(len(el.FMask.Data()), el.FMask.Data())
		el.Fx = utils.NewMatrix(3*el.Nfp, el.K)
		for fp, val := range el.FMask.Data() {
			ind := int(val)
			el.Fx.M.SetRow(fp, el.X.M.RawRowView(ind))
		}
		el.Fy = utils.NewMatrix(3*el.Nfp, el.K)
		for fp, val := range el.FMask.Data() {
			ind := int(val)
			el.Fy.M.SetRow(fp, el.Y.M.RawRowView(ind))
		}
		el.Lift2D()
	}
	el.GeometricFactors2D()
	el.Normals2D()
	el.FScale = el.sJ.ElDiv(el.J.Subset(el.GetFaces()))
	// Build connectivity matrix
	el.Connect2D()

	// Mark fields read only
	el.Dr.SetReadOnly("Dr")
	el.Ds.SetReadOnly("Ds")
	el.LIFT.SetReadOnly("LIFT")
	el.X.SetReadOnly("X")
	el.Y.SetReadOnly("Y")
	el.Fx.SetReadOnly("Fx")
	el.Fy.SetReadOnly("Fy")
	el.FMask.SetReadOnly("FMask")
	el.MassMatrix.SetReadOnly("MassMatrix")
	el.V.SetReadOnly("V")
	el.Vinv.SetReadOnly("Vinv")
	el.NX.SetReadOnly("NX")
	el.NY.SetReadOnly("NY")
	el.FScale.SetReadOnly("FScale")
	el.EToE.SetReadOnly("EToE")
	el.EToF.SetReadOnly("EToF")
	return
}

/*
	Startup2D
  // Build connectivity maps
  BuildMaps2D();
  // Compute weak operators (could be done in preprocessing to save time)
  DMat Vr,Vs;  GradVandermonde2D(N, r, s, Vr, Vs);
  VVT = V*trans(V);
  Drw = (V*trans(Vr))/VVT;  Dsw = (V*trans(Vs))/VVT;
*/
func (el *Elements2D) Connect2D() {
	var (
		Nv         = el.VX.Len()
		TotalFaces = el.NFaces * el.K
	)
	SpFToVDOK := utils.NewDOK(TotalFaces, Nv)
	faces := utils.NewMatrix(3, 2, []float64{
		0, 1,
		1, 2,
		0, 2,
	})
	var sk int
	for k := 0; k < el.K; k++ {
		for face := 0; face < el.NFaces; face++ {
			edge := faces.Range(face, ":")
			//fmt.Println("Nv, TotalFaces, k, face, edge, range = ", Nv, TotalFaces, k, face, edge, el.EToV.Range(k, edge))
			SpFToVDOK.Equate(1, sk, el.EToV.Range(k, edge))
			sk++
		}
	}
	// Build global face to global face sparse array
	SpFToV := SpFToVDOK.ToCSR()
	SpFToF := utils.NewCSR(TotalFaces, TotalFaces)
	SpFToF.M.Mul(SpFToV, SpFToV.T())
	for i := 0; i < TotalFaces; i++ {
		SpFToF.M.Set(i, i, SpFToF.At(i, i)-2)
	}
	// Find complete face to face connections
	F12 := utils.MatFind(SpFToF, utils.Equal, 2)

	element1 := F12.RI.Copy().Apply(func(val int) int { return val / el.NFaces })
	face1 := F12.RI.Copy().Apply(func(val int) int { return int(math.Mod(float64(val), float64(el.NFaces))) })

	element2 := F12.CI.Copy().Apply(func(val int) int { return val / el.NFaces })
	face2 := F12.CI.Copy().Apply(func(val int) int { return int(math.Mod(float64(val), float64(el.NFaces))) })

	// Rearrange into Nelements x Nfaces sized arrays
	el.EToE = utils.NewRangeOffset(1, el.K).Outer(utils.NewOnes(el.NFaces))
	el.EToF = utils.NewOnes(el.K).Outer(utils.NewRangeOffset(1, el.NFaces))
	var I2D utils.Index2D
	var err error
	nr, nc := el.EToE.Dims()
	if I2D, err = utils.NewIndex2D(nr, nc, element1, face1); err != nil {
		panic(err)
	}
	el.EToE.Assign(I2D.ToIndex(), element2)
	el.EToF.Assign(I2D.ToIndex(), face2)
}

func (el *Elements2D) GetFaces() (aI utils.Index, NFacePts, K int) {
	var (
		err      error
		allFaces utils.Index2D
	)
	NFacePts = el.Nfp * el.NFaces
	K = el.K
	allK := utils.NewRangeOffset(1, el.K)
	if allFaces, err = utils.NewIndex2D(el.Np, el.K, el.FMaskI, allK, true); err != nil {
		panic(err)
	}
	aI = allFaces.ToIndex()
	return
}

func (el *Elements2D) Normals2D() {
	var (
		f1, f2, f3 utils.Index2D
		err        error
	)
	allK := utils.NewRangeOffset(1, el.K)
	aI, NFacePts, _ := el.GetFaces()
	// interpolate geometric factors to face nodes
	fxr := el.xr.Subset(aI, NFacePts, el.K)
	fxs := el.xs.Subset(aI, NFacePts, el.K)
	fyr := el.yr.Subset(aI, NFacePts, el.K)
	fys := el.ys.Subset(aI, NFacePts, el.K)
	// build normals
	faces1 := utils.NewRangeOffset(1, el.Nfp)
	faces2 := utils.NewRangeOffset(1+el.Nfp, 2*el.Nfp)
	faces3 := utils.NewRangeOffset(1+2*el.Nfp, 3*el.Nfp)
	if f1, err = utils.NewIndex2D(NFacePts, el.K, faces1, allK, true); err != nil {
		panic(err)
	}
	if f2, err = utils.NewIndex2D(NFacePts, el.K, faces2, allK, true); err != nil {
		panic(err)
	}
	if f3, err = utils.NewIndex2D(NFacePts, el.K, faces3, allK, true); err != nil {
		panic(err)
	}
	el.NX, el.NY = utils.NewMatrix(NFacePts, el.K), utils.NewMatrix(NFacePts, el.K)
	// Face 1
	el.NX.Assign(f1.ToIndex(), fyr.Subset(f1.ToIndex(), f1.Len, 1))
	el.NY.Assign(f1.ToIndex(), fxr.Subset(f1.ToIndex(), f1.Len, 1).Scale(-1))
	// Face 2
	el.NX.Assign(f2.ToIndex(), fys.Subset(f2.ToIndex(), f2.Len, 1).Subtract(fyr.Subset(f2.ToIndex(), f2.Len, 1)))
	el.NY.Assign(f2.ToIndex(), fxs.Subset(f2.ToIndex(), f2.Len, 1).Scale(-1).Add(fxr.Subset(f2.ToIndex(), f2.Len, 1)))
	// Face 3
	el.NX.Assign(f3.ToIndex(), fys.Subset(f3.ToIndex(), f3.Len, 1).Scale(-1))
	el.NY.Assign(f3.ToIndex(), fxs.Subset(f3.ToIndex(), f3.Len, 1))
	el.sJ = el.NX.Copy().POW(2).Add(el.NY.Copy().POW(2)).Apply(func(val float64) (res float64) {
		res = math.Sqrt(val)
		return
	})
	el.NX.ElDiv(el.sJ)
	el.NY.ElDiv(el.sJ)
}

func (el *Elements2D) GeometricFactors2D() {
	/*
	  // function [rx,sx,ry,sy,J] = GeometricFactors2D(x,y,Dr,Ds)
	  // Purpose  : Compute the metric elements for the local
	  //            mappings of the elements
	  DMat xr=Dr*x
	       xs=Ds*x
	       yr=Dr*y
	       ys=Ds*y;
	  J  =  xr.dm(ys) - xs.dm(yr);
	  rx = ys.dd(J); sx = -yr.dd(J); ry = -xs.dd(J); sy = xr.dd(J);
	*/
	// Calculate geometric factors
	el.xr, el.xs = el.Dr.Mul(el.X), el.Ds.Mul(el.X)
	el.yr, el.ys = el.Dr.Mul(el.Y), el.Ds.Mul(el.Y)
	el.xr.SetReadOnly("xr")
	el.xs.SetReadOnly("xs")
	el.yr.SetReadOnly("yr")
	el.ys.SetReadOnly("ys")
	el.J = el.xr.Copy().ElMul(el.ys).Subtract(el.xs.Copy().ElMul(el.yr))
	el.Rx = el.ys.Copy().ElDiv(el.J)
	el.Sx = el.yr.Copy().ElDiv(el.J).Scale(-1)
	el.Ry = el.xs.Copy().ElDiv(el.J).Scale(-1)
	el.Sy = el.xr.Copy().ElDiv(el.J)
}

func (el *Elements2D) Lift2D() {
	var (
		err      error
		I2       utils.Index2D
		massEdge utils.Matrix
		V1D      utils.Matrix
		Emat     utils.Matrix
	)
	Emat = utils.NewMatrix(el.Np, el.NFaces*el.Nfp)
	faceMap := func(basis utils.Vector, faceNum int, Ind utils.Index) {
		faceBasis := basis.SubsetIndex(el.FMask.Col(faceNum).ToIndex())
		V1D = DG1D.Vandermonde1D(el.N, faceBasis)
		if massEdge, err = V1D.Mul(V1D.Transpose()).Inverse(); err != nil {
			panic(err)
		}
		if I2, err = utils.NewIndex2D(el.Np, el.NFaces*el.Nfp, el.FMask.Col(faceNum).ToIndex(), Ind, true); err != nil {
			panic(err)
		}
		Emat.Assign(I2.ToIndex(), massEdge)
	}
	// face 1
	faceMap(el.R, 0, utils.NewRangeOffset(1, el.Nfp))
	// face 2
	faceMap(el.R, 1, utils.NewRangeOffset(el.Nfp+1, 2*el.Nfp))
	// face 3
	faceMap(el.S, 2, utils.NewRangeOffset(2*el.Nfp+1, 3*el.Nfp))
	// inv(mass matrix)*\I_n (L_i,L_j)_{edge_n}
	el.LIFT = el.V.Mul(el.V.Transpose().Mul(Emat))
	return
}

func GradVandermonde2D(N int, r, s utils.Vector) (V2Dr, V2Ds utils.Matrix) {
	var (
		a, b = RStoAB(r, s)
		Np   = (N + 1) * (N + 2) / 2
		Nr   = r.Len()
	)
	V2Dr, V2Ds = utils.NewMatrix(Nr, Np), utils.NewMatrix(Nr, Np)
	var sk int
	for i := 0; i <= N; i++ {
		for j := 0; j <= (N - i); j++ {
			ddr, dds := GradSimplex2DP(a, b, i, j)
			V2Dr.M.SetCol(sk, ddr)
			V2Ds.M.SetCol(sk, dds)
			sk++
		}
	}
	return
}

func GradSimplex2DP(a, b utils.Vector, id, jd int) (ddr, dds []float64) {
	var (
		ad, bd = a.Data(), b.Data()
	)
	_ = ad
	fa := DG1D.JacobiP(a, 0, 0, id)
	dfa := DG1D.GradJacobiP(a, 0, 0, id)
	gb := DG1D.JacobiP(b, 2*float64(id)+1, 0, jd)
	dgb := DG1D.GradJacobiP(b, 2*float64(id)+1, 0, jd)
	// r-derivative
	// d/dr = da/dr d/da + db/dr d/db = (2/(1-s)) d/da = (2/(1-b)) d/da
	ddr = make([]float64, len(gb))
	for i := range ddr {
		ddr[i] = dfa[i] * gb[i]
		if id > 0 {
			ddr[i] *= utils.POW(0.5*(1-bd[i]), id-1)
		}
		// Normalize
		ddr[i] *= math.Pow(2, float64(id)+0.5)
	}
	// s-derivative
	// d/ds = ((1+a)/2)/((1-b)/2) d/da + d/db
	dds = make([]float64, len(gb))
	for i := range dds {
		dds[i] = 0.5 * dfa[i] * gb[i] * (1 + ad[i])
		if id > 0 {
			dds[i] *= utils.POW(0.5*(1-bd[i]), id-1)
		}
		tmp := dgb[i] * utils.POW(0.5*(1-bd[i]), id)
		if id > 0 {
			tmp -= 0.5 * float64(id) * gb[i] * utils.POW(0.5*(1-bd[i]), id-1)
		}
		dds[i] += fa[i] * tmp
		// Normalize
		dds[i] *= math.Pow(2, float64(id)+0.5)
	}
	return
}

func (el *Elements2D) BuildMaps2D() {
	return
}

func (el *Elements2D) NewCube2D(COrder int) {
	// function [cubR,cubS,cubW, Ncub] = Cubature2D(COrder)
	// Purpose: provide multidimensional quadrature (i.e. cubature)
	//          rules to integrate up to COrder polynomials

	if COrder > 28 {
		COrder = 28
	}

	if COrder <= 28 {
		cub2d := getCub(COrder)
		nr := len(cub2d) / 3
		cubMat := utils.NewMatrix(nr, 3, cub2d)
		el.Cub = &Cubature{
			r: cubMat.Col(0),
			s: cubMat.Col(1),
			w: cubMat.Col(2),
		}
	} else {
		err := fmt.Errorf("Cubature2D(%d): COrder > 28 not yet tested\n", COrder)
		panic(err)
		/*
		   DVec cuba,cubwa, cubb,cubwb
		   DMat cubA, cubB, cubR, cubS, cubW, tA,tB

		   int cubNA=(int)ceil((COrder+1.0)/2.0)
		   int cubNB=(int)ceil((COrder+1.0)/2.0)


		   JacobiGQ(1.0, 0.0, cubNB-1,  cubb,cubwb)

		   cubA = outer( ones(cubNB), cuba )
		   cubB = outer( cubb, ones(cubNA) )

		   tA = 1.0+cubA
		   tB = 1.0-cubB
		   cubR = 0.5 * tA.dm(tB) - 1.0
		   cubS = cubB
		   cubW = 0.5 * outer(cubwb, cubwa)

		   cub.r = cubR
		   cub.s = cubS
		   cub.w = cubW
		   cub.Ncub = cub.r.size()
		*/
	}
	return
}

func RStoAB(r, s utils.Vector) (a, b utils.Vector) {
	var (
		Np     = r.Len()
		rd, sd = r.Data(), s.Data()
	)
	ad, bd := make([]float64, Np), make([]float64, Np)
	for n, sval := range sd {
		if sval != 1 {
			ad[n] = 2*(1+rd[n])/(1-sval) - 1
		} else {
			ad[n] = -1
		}
		bd[n] = sval
	}
	a, b = utils.NewVector(Np, ad), utils.NewVector(Np, bd)
	return
}

// function [r,s] = xytors(x,y)
// Purpose : Transfer from (x,y) in equilateral triangle
//           to (r,s) coordinates in standard triangle
func XYtoRS(x, y utils.Vector) (r, s utils.Vector) {
	r, s = utils.NewVector(x.Len()), utils.NewVector(x.Len())
	var (
		xd, yd = x.Data(), y.Data()
		rd, sd = r.Data(), s.Data()
	)
	sr3 := math.Sqrt(3)
	for i := range xd {
		l1 := (sr3*yd[i] + 1) / 3
		l2 := (-3*xd[i] - sr3*yd[i] + 2) / 6
		l3 := (3*xd[i] - sr3*yd[i] + 2) / 6
		rd[i] = -l2 + l3 - l1
		sd[i] = -l2 - l3 + l1
	}
	return
}

func JacobiP2D(u, v float64, i, j int) (p float64) {
	/*
		This is a 2D normalized polynomial basis for the unit triangle with vertices:
					(0,0), (0,1), (1, 0)
		Arguments:
					(u,v) is the position within the triangle where the basis is evaluated
		    		(i,j) is the index within the basis, which has (K+1)(K+2)/2 terms
	*/
	a := utils.NewVector(1, []float64{2*u/(1-v) - 1})
	b := utils.NewVector(1, []float64{2*v - 1})
	h1 := DG1D.JacobiP(a, 0, 0, i)
	h2 := DG1D.JacobiP(b, float64(2*i+1), 0, i)
	p = math.Sqrt(2) * utils.POW(2*(1-v), j) * h1[0] * h2[0] / 4 // The divide by 4 is the Jacobian that converts from a different triangle
	return
}
