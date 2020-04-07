package DG1D

import (
	"fmt"
	"os"

	"github.com/notargets/gocfd/utils"
	"gonum.org/v1/gonum/mat"
)

func SimpleMesh1D(xmin, xmax float64, K int) (VX *mat.VecDense, EToV *mat.Dense) {
	// K is the number of elements, there are K+1 vertices
	var (
		x             = make([]float64, K+1)
		elementVertex = make([]float64, K*2)
	)
	for i := 0; i < K+1; i++ {
		x[i] = (xmax-xmin)*float64(i)/float64(K) + xmin
	}
	var iter int
	for i := 0; i < K; i++ {
		elementVertex[iter] = float64(i)
		elementVertex[iter+1] = float64(i + 1)
		iter += 2
	}
	return mat.NewVecDense(K+1, x), mat.NewDense(K, 2, elementVertex)
}

func Startup1D(K, N, NFaces, Nfp int) (X *mat.Dense) {
	var (
		Np = N + 1
	)
	VX, EToV := SimpleMesh1D(0, 2, K)

	_, R, W := JacobiGL(0, 0, N)
	V := Vandermonde1D(N, R)
	Vinv := mat.NewDense(Np, Np, nil)
	if err := Vinv.Inverse(V); err != nil {
		panic("error inverting V")
	}
	Vr := GradVandermonde1D(R, N)
	Dr := mat.NewDense(Np, Np, nil)
	Dr.Product(Vr, Vinv)
	LIFT := Lift1D(V, Np, NFaces, Nfp)

	NX := Normals1D(NFaces, Nfp, K)

	va := EToV.ColView(0)
	vb := EToV.ColView(1)
	sT := mat.NewVecDense(va.Len(), nil)
	sT.SubVec(utils.VecSub(VX, vb), utils.VecSub(VX, va))

	// x = ones(Np)*VX(va) + 0.5*(r+1.)*sT(vc);
	ones := utils.NewVecConst(Np, 1)
	mm := mat.NewDense(Np, K, nil)
	mm.Mul(ones, utils.VecSubV(VX, va).T())

	rr := utils.VecScalarAdd(mat.VecDenseCopyOf(R), 1)
	rr.ScaleVec(0.5, rr)

	X = mat.NewDense(Np, K, nil)
	X.Mul(rr, sT.T())
	X.Add(X, mm)

	rrr := utils.Vector{rr}.ToMatrix()
	ssT := utils.Vector{sT}.Transpose()
	mmm := utils.Matrix{mm}
	XX := rrr.Mul(ssT).Add(mmm)
	fmt.Printf("X = \n%v\nXX = \n%v\n", mat.Formatted(X, mat.Squeeze()), mat.Formatted(XX.M, mat.Squeeze()))
	os.Exit(1)

	J, Rx := GeometricFactors1D(Dr, X)

	fmask1 := utils.VecFind(utils.VecScalarAdd(R, 1), utils.Less, utils.NODETOL, true)
	fmask2 := utils.VecFind(utils.VecScalarAdd(R, -1), utils.Less, utils.NODETOL, true)
	FMask := utils.VecConcat(fmask1, fmask2)
	Fx := utils.MatSubsetRow(X, FMask)
	JJ := utils.MatSubsetRow(J, FMask)
	FScale := utils.MatElementInvert(JJ)

	EToE, EToF := Connect1D(EToV)

	vmapM, vmapP, mapB, vmapB, mapI, vmapI, mapO, vmapO :=
		BuildMaps1D(VX, FMask,
			X, EToV, EToE, EToF,
			K, Np, Nfp, NFaces,
			utils.NODETOL)
	_, _, _, _, _, _ = W, LIFT, NX, Rx, Fx, FScale
	_, _, _, _, _, _, _, _ = vmapM, vmapP, mapB, vmapB, mapI, vmapI, mapO, vmapO
	return
}