package utils

import (
	"fmt"

	"gonum.org/v1/gonum/lapack/lapack64"

	"gonum.org/v1/gonum/blas/blas64"

	"gonum.org/v1/gonum/mat"
)

type Matrix struct {
	M        *mat.Dense
	readOnly bool
	name     string
}

func NewMatrix(nr, nc int, dataO ...[]float64) Matrix {
	var (
		data []float64
	)
	if len(dataO) != 0 {
		data = dataO[0]
	}
	return Matrix{
		mat.NewDense(nr, nc, data),
		false,
		"unnamed - hint: pass a variable name to SetReadOnly()",
	}
}

// Dims, At and T minimally satisfy the mat.Matrix interface.
func (m Matrix) Dims() (r, c int)          { return m.M.Dims() }
func (m Matrix) At(i, j int) float64       { return m.M.At(i, j) }
func (m Matrix) T() mat.Matrix             { return m.T() }
func (m Matrix) RawMatrix() blas64.General { return m.M.RawMatrix() }

// Chainable methods (extended)
func (m *Matrix) SetReadOnly(name ...string) Matrix {
	if len(name) != 0 {
		m.name = name[0]
	}
	m.readOnly = true
	return *m
}

func (m *Matrix) SetWritable() Matrix {
	m.readOnly = false
	return *m
}

func (m Matrix) Slice(I, K, J, L int) (R Matrix) { // Does not change receiver
	var (
		nrR   = K - I
		ncR   = L - J
		dataR = make([]float64, nrR*ncR)
		nr, _ = m.Dims()
		data  = m.M.RawMatrix().Data
	)
	for j := J; j < L; j++ {
		for i := I; i < K; i++ {
			ind := i + nr*j
			iR := i - I
			jR := j - J
			indR := iR + nrR*jR
			dataR[indR] = data[ind]
		}
	}
	R = NewMatrix(nrR, ncR, dataR)
	return
}

func (m Matrix) Copy() (R Matrix) { // Does not change receiver
	var (
		data   = m.M.RawMatrix().Data
		nr, nc = m.Dims()
		dataR  = make([]float64, nr*nc)
	)
	for i, val := range data {
		dataR[i] = val
	}
	R = NewMatrix(nr, nc, dataR)
	return
}

func (m Matrix) Transpose() (R Matrix) { // Does not change receiver
	var (
		nr, nc = m.Dims()
		data   = m.M.RawMatrix().Data
	)
	R = NewMatrix(nc, nr)
	dataR := R.M.RawMatrix().Data
	for j := 0; j < nc; j++ {
		for i := 0; i < nr; i++ {
			ind := i + nr*j
			indR := i*nc + j
			dataR[ind] = data[indR]
		}
	}
	return
}

func (m Matrix) Mul(A Matrix) (R Matrix) { // Does not change receiver
	var (
		nrM, _ = m.M.Dims()
		_, ncA = A.M.Dims()
	)
	R = NewMatrix(nrM, ncA)
	R.M.Mul(m.M, A.M)
	return R
}

func (m Matrix) Subset(I Index, nrNew, ncNew int) (R Matrix) { // Does not change receiver
	/*
		Index should contain a list of indices into MI
		Note: native mat library matrix storage is in column traversal first (row-major) order
	*/
	var (
		Mr     = m.RawMatrix()
		nr, nc = m.Dims()
		data   = make([]float64, nrNew*ncNew)
	)
	for i, ind := range I {
		indC := RowMajorToColMajor(nr, nc, ind)
		indD := RowMajorToColMajor(nrNew, ncNew, i)
		data[indD] = Mr.Data[indC]
	}
	return NewMatrix(nrNew, ncNew, data)
}

func (m Matrix) SliceRows(I Index) (R Matrix) { // Does not change receiver
	// RowIndices should contain a list of row indices into M
	var (
		nr, nc = m.Dims()
		nI     = len(I)
	)
	R = NewMatrix(nI, nc)
	for i, val := range I {
		if val > nr-1 || val < 0 {
			fmt.Printf("index out of bounds: index = %d, max_bounds = %d\n", val, nr-1)
			panic("unable to subset row from matrix")
		}
		R.M.SetRow(i, m.M.RawRowView(val))
	}
	return
}

func (m Matrix) Set(i, j int, val float64) Matrix { // Changes receiver
	m.checkWritable()
	m.M.Set(i, j, val)
	return m
}

func (m Matrix) SetCol(j int, data []float64) Matrix { // Changes receiver
	m.checkWritable()
	m.M.SetCol(j, data)
	return m
}

func (m Matrix) Add(A Matrix) Matrix { // Changes receiver
	var (
		dataM = m.RawMatrix().Data
		dataA = A.RawMatrix().Data
	)
	m.checkWritable()
	for i, val := range dataA {
		dataM[i] += val
	}
	return m
}

func (m Matrix) Subtract(a Matrix) Matrix { // Changes receiver
	var (
		data  = m.M.RawMatrix().Data
		dataA = a.M.RawMatrix().Data
	)
	m.checkWritable()
	for i := range data {
		data[i] -= dataA[i]
	}
	return m
}
func (m Matrix) Assign(I Index, A Matrix) Matrix { // Changes receiver
	// Assigns values in M sequentially using values indexed from A
	var (
		nr, nc = m.Dims()
		dataM  = m.RawMatrix().Data
		dataA  = A.RawMatrix().Data
	)
	m.checkWritable()
	for _, ind := range I {
		i := RowMajorToColMajor(nr, nc, ind)
		dataM[i] = dataA[i]
	}
	return m
}

func (m Matrix) Scale(a float64) Matrix { // Changes receiver
	var (
		data = m.M.RawMatrix().Data
	)
	m.checkWritable()
	for i := range data {
		data[i] *= a
	}
	return m
}

func (m Matrix) AddScalar(a float64) Matrix { // Changes receiver
	var (
		data = m.M.RawMatrix().Data
	)
	m.checkWritable()
	for i := range data {
		data[i] += a
	}
	return m
}

func (m Matrix) Apply(f func(float64) float64) Matrix { // Changes receiver
	var (
		data = m.M.RawMatrix().Data
	)
	m.checkWritable()
	for i, val := range data {
		data[i] = f(val)
	}
	return m
}

func (m Matrix) POW(p int) Matrix { // Changes receiver
	var (
		data = m.M.RawMatrix().Data
	)
	m.checkWritable()
	for i, val := range data {
		data[i] = POW(val, p)
	}
	return m
}

func (m Matrix) ElementMultiply(A Matrix) Matrix { // Changes receiver
	var (
		dataM = m.RawMatrix().Data
		dataA = A.RawMatrix().Data
	)
	m.checkWritable()
	for i, val := range dataA {
		//fmt.Printf("val[%d] = %v, mval = %v\n", i, val, dataM[i])
		dataM[i] *= val
	}
	return m
}

func (m Matrix) AssignScalar(I Index, val float64) Matrix { // Changes receiver
	var (
		dataM  = m.RawMatrix().Data
		nr, nc = m.Dims()
	)
	m.checkWritable()
	for _, ind := range I {
		i := RowMajorToColMajor(nr, nc, ind)
		dataM[i] = val
	}
	return m
}

// Non chainable methods
func (m Matrix) IndexedAssign(I2 Index2D, Val Index) (err error) { // Changes receiver
	var (
		data = m.RawMatrix().Data
	)
	m.checkWritable()
	if I2.Len != len(Val) {
		err = fmt.Errorf("length of index and values are not equal: len(I2) = %v, len(Val) = %v\n", I2.Len, len(Val))
		return
	}
	for i, val := range Val {
		data[I2.Ind[i]] = float64(val)
	}
	return
}

func (m Matrix) Inverse() (R Matrix, err error) {
	var (
		nr, nc = m.Dims()
	)
	R = m.Copy()
	iPiv := make([]int, nr)
	if ok := lapack64.Getrf(R.RawMatrix(), iPiv); !ok {
		err = fmt.Errorf("unable to invert, matrix is singular")
		return
	}
	work := make([]float64, nr*nc)
	if ok := lapack64.Getri(R.RawMatrix(), iPiv, work, nr*nc); !ok {
		err = fmt.Errorf("unable to invert, matrix is singular")
	}
	return
}

func (m Matrix) Col(j int) Vector {
	var (
		data   = m.M.RawMatrix().Data
		nr, nc = m.M.Dims()
		vData  = make([]float64, nr)
	)
	for i := range vData {
		vData[i] = data[i*nc+j]
	}
	return NewVector(nr, vData)
}

func (m Matrix) Row(i int) Vector {
	var (
		data  = m.M.RawMatrix().Data
		_, nc = m.M.Dims()
		vData = make([]float64, nc)
	)
	for j := range vData {
		vData[j] = data[j+i*nc]
	}
	return NewVector(nc, vData)
}

func (m Matrix) Min() (min float64) {
	var (
		data = m.M.RawMatrix().Data
	)
	min = data[0]
	for _, val := range data {
		if val < min {
			min = val
		}
	}
	return
}

func (m Matrix) Max() (max float64) {
	var (
		data = m.M.RawMatrix().Data
	)
	max = data[0]
	for _, val := range data {
		if val > max {
			max = val
		}
	}
	return
}

func (m Matrix) Find(op EvalOp, val float64) (I Index2D) {
	var (
		nr, nc         = m.Dims()
		data           = m.RawMatrix().Data
		rowInd, colInd Index
	)
	switch op {
	case Equal:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				ind := i + nr*j
				if data[ind] == val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	case Less:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				ind := i + nr*j
				if data[ind] < val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	case LessOrEqual:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				ind := i + nr*j
				if data[ind] <= val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	case Greater:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				ind := i + nr*j
				if data[ind] > val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	case GreaterOrEqual:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				ind := i + nr*j
				if data[ind] >= val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	}
	I, _ = NewIndex2D(nr, nc, rowInd, colInd)
	return
}

func (m Matrix) SubsetVector(I Index) (V Vector) {
	var (
		Mr     = m.RawMatrix()
		nr, nc = m.Dims()
		data   = make([]float64, len(I))
	)
	for i, ind := range I {
		data[i] = Mr.Data[RowMajorToColMajor(nr, nc, ind)]
	}
	V = NewVector(len(I), data)
	return
}

func (m Matrix) checkWritable() {
	if m.readOnly {
		err := fmt.Errorf("attempt to write to a read only matrix named: \"%v\"", m.name)
		panic(err)
	}
}

func RowMajorToColMajor(nr, nc, ind int) (cind int) {
	// ind = i + nr * j
	// ind / nr = 0 + j
	j := ind / nr
	i := ind - nr*j
	cind = j + nc*i
	return
}
