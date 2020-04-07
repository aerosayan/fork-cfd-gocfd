package utils

import (
	"fmt"

	"gonum.org/v1/gonum/blas/blas64"

	"gonum.org/v1/gonum/mat"
)

type Matrix struct {
	M *mat.Dense
}

// Dims, At and T minimally satisfy the mat.Matrix interface.
func (m Matrix) Dims() (r, c int)          { return m.M.Dims() }
func (m Matrix) At(i, j int) float64       { return m.M.At(i, j) }
func (m Matrix) T() mat.Matrix             { return m.T() }
func (m Matrix) RawMatrix() blas64.General { return m.M.RawMatrix() }

// Chainable methods (extended)
func (m Matrix) Sub(a Matrix) Matrix { m.M.Sub(m.M, a.M); return m }

func (m Matrix) SubsetVector(I Index) Vector {
	return Vector{MatSubset(m.M, I)}
}

func (m Matrix) Subset(I Index) Matrix {
	/*
		Index should contain a list of indices into MI
		Note: native mat library matrix storage is in column traversal first (row-major) order
	*/
	var (
		Mr     = m.RawMatrix()
		nr, nc = m.Dims()
		data   = make([]float64, nr*nc)
		R      *mat.Dense
	)
	for _, ind := range I {
		data[ind] = Mr.Data[ind]
	}
	R = mat.NewDense(nr, nc, data)
	return Matrix{R}
}

func (m Matrix) Add(a float64) Matrix {
	var (
		data = m.M.RawMatrix().Data
	)
	for i := range data {
		data[i] += a
	}
	return m
}

func (m Matrix) Apply(f func(float64) float64) Matrix {
	var (
		data = m.M.RawMatrix().Data
	)
	for i, val := range data {
		data[i] = f(val)
	}
	return m
}

func (m Matrix) POW(p int) Matrix {
	var (
		data = m.M.RawMatrix().Data
	)
	for i, val := range data {
		data[i] = POW(val, p)
	}
	return m
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
	return MatFind(m.M, op, val)
}

func MatElementInvert(M mat.Matrix) (R *mat.Dense) {
	var (
		d, s = M.Dims()
	)
	R = mat.NewDense(d, s, nil)
	R.CloneFrom(M)
	for j := 0; j < s; j++ {
		for i := 0; i < d; i++ {
			R.Set(i, j, 1./R.At(i, j))
		}
	}
	return
}

func MatCopyEmpty(M *mat.Dense) (R *mat.Dense) {
	var (
		nr, nc = M.Dims()
	)
	R = mat.NewDense(nr, nc, nil)
	return
}

func MatSubset(M *mat.Dense, I Index) (r *mat.VecDense) {
	/*
		Index should contain a list of indices into MI
		Note: native mat library matrix storage is in column traversal first (row-major) order
	*/
	var (
		Mr     = M.RawMatrix()
		nr, nc = M.Dims()
		data   = make([]float64, len(I))
	)
	for i, ind := range I {
		data[i] = Mr.Data[RowMajorToColMajor(nr, nc, ind)]
	}
	r = mat.NewVecDense(len(I), data)
	return
}

func RowMajorToColMajor(nr, nc, ind int) (cind int) {
	// ind = i + nr * j
	// ind / nr = 0 + j
	j := ind / nr
	i := ind - nr*j
	cind = j + nc*i
	return
}

func MatSubsetRow(MI mat.Matrix, RowIndices *mat.VecDense) (R *mat.Dense) {
	// RowIndices should contain a list of row indices into M
	var (
		nr, nc = MI.Dims()
		nrI    = RowIndices.Len()
		rI     = RowIndices.RawVector().Data
	)
	R = mat.NewDense(nrI, nc, nil)
	for i, val := range rI {
		valI := int(val)
		if valI > nr-1 || valI < 0 {
			fmt.Printf("index out of bounds: index = %d, max_bounds = %d\n", valI, nr-1)
			panic("unable to subset row from matrix")
		}
		var rowSlice []float64
		if M, ok := MI.(*mat.Dense); ok {
			rowSlice = M.RawRowView(valI)
		} else {
			rowSlice = make([]float64, nc)
			for j := 0; j < nc; j++ {
				rowSlice[j] = MI.At(i, j)
			}
		}
		R.SetRow(i, rowSlice)
	}
	return
}

func MatSubsetCol(MI mat.Matrix, ColIndices *mat.VecDense) (R *mat.Dense) {
	// ColIndices should contain a list of row indices into M
	var (
		nr, nc = MI.Dims()
		ncI    = ColIndices.Len()
		cI     = ColIndices.RawVector().Data
	)
	R = mat.NewDense(nr, ncI, nil)
	for j, val := range cI {
		valI := int(val)
		if valI > nc-1 || valI < 0 {
			panic("unable to subset row from matrix, index out of bounds")
		}
		var colSlice []float64
		if M, ok := MI.(*mat.Dense); ok {
			colSlice = VecGetF64(M.ColView(valI))
		} else {
			colSlice = make([]float64, nr)
			for i := 0; i < nr; i++ {
				colSlice[i] = MI.At(i, j)
			}
		}
		R.SetCol(j, colSlice)
	}
	return
}

func MatFind(MI mat.Matrix, op EvalOp, val float64) (I Index2D) {
	var (
		nr, nc         = MI.Dims()
		rowInd, colInd Index
	)
	switch op {
	case Equal:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				if MI.At(i, j) == val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	case Less:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				if MI.At(i, j) < val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	case LessOrEqual:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				if MI.At(i, j) <= val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	case Greater:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				if MI.At(i, j) > val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	case GreaterOrEqual:
		for j := 0; j < nc; j++ {
			for i := 0; i < nr; i++ {
				if MI.At(i, j) >= val {
					rowInd = append(rowInd, i)
					colInd = append(colInd, j)
				}
			}
		}
	}
	I, _ = NewIndex2D(nr, nc, rowInd, colInd)
	return
}

func MatIndexedAssign(MI *mat.Dense, I2 Index2D, Val Index) (err error) {
	var (
		data = MI.RawMatrix().Data
	)
	if I2.Len != len(Val) {
		return fmt.Errorf("length of index and values are not equal: len(I2) = %v, len(Val) = %v\n", I2.Len, len(Val))
	}
	for i, val := range Val {
		data[I2.Ind[i]] = float64(val)
	}
	return
}

func MatApply(M *mat.Dense, f func(x float64) float64) (R *mat.Dense) {
	var (
		nr, nc = M.Dims()
		data   = M.RawMatrix().Data
	)
	R = mat.NewDense(nr, nc, nil)
	newData := R.RawMatrix().Data
	for i, val := range data {
		newData[i] = f(val)
	}
	return
}

func MatApplyInPlace(M *mat.Dense, f func(x float64) float64) {
	var (
		data = M.RawMatrix().Data
	)
	for i, val := range data {
		data[i] = f(val)
	}
}

func MatPOWInPlace(M *mat.Dense, p int) {
	var (
		data = M.RawMatrix().Data
	)
	for i, val := range data {
		data[i] = POW(val, p)
	}
}
