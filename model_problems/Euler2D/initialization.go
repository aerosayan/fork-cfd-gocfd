package Euler2D

import (
	"fmt"
	"strings"

	"github.com/notargets/gocfd/model_problems/Euler2D/isentropic_vortex"
	"github.com/notargets/gocfd/utils"
)

type ExactState interface {
	GetStateC(t, x, y float64) (rho, rhoU, rhoV, E float64)
	GetDivergence(t, x, y float64) (div [4]float64)
}

type InitType uint

const (
	FREESTREAM InitType = iota
	IVORTEX
)

var (
	InitNames = map[string]InitType{
		"freestream": FREESTREAM,
		"ivortex":    IVORTEX,
	}
	InitPrintNames = []string{"Freestream", "Inviscid Vortex Analytic Solution"}
)

func NewInitType(label string) (it InitType) {
	var (
		ok  bool
		err error
	)
	if len(label) == 0 {
		err = fmt.Errorf("empty init type, must be one of %v", InitNames)
		panic(err)
	}
	label = strings.ToLower(label)
	if it, ok = InitNames[label]; !ok {
		err = fmt.Errorf("unable to use init type named %s", label)
		panic(err)
	}
	return
}

func (c *Euler) InitializeFS(Kmax int) (Q [4]utils.Matrix) {
	var (
		Np = c.dfr.SolutionElement.Np
	)
	Q[0] = utils.NewMatrix(Np, Kmax).AddScalar(c.FS.Qinf[0])
	Q[1] = utils.NewMatrix(Np, Kmax).AddScalar(c.FS.Qinf[1])
	Q[2] = utils.NewMatrix(Np, Kmax).AddScalar(c.FS.Qinf[2])
	Q[3] = utils.NewMatrix(Np, Kmax).AddScalar(c.FS.Qinf[3])
	return
}

func (c *Euler) InitializeIVortex(X, Y utils.Matrix) (iv *isentropic_vortex.IVortex, Q [4]utils.Matrix) {
	var (
		Beta     = 5.
		X0, Y0   = 5., 0.
		Gamma    = 1.4
		Np, Kmax = X.Dims()
	)
	for n := 0; n < 4; n++ {
		Q[n] = utils.NewMatrix(Np, Kmax)
	}
	iv = isentropic_vortex.NewIVortex(Beta, X0, Y0, Gamma)
	qD := Get4DP(Q)
	xD, yD := X.Data(), Y.Data()
	for ii := 0; ii < Np*Kmax; ii++ {
		x, y := xD[ii], yD[ii]
		rho, rhoU, rhoV, E := iv.GetStateC(0, x, y)
		qD[0][ii] = rho
		qD[1][ii] = rhoU
		qD[2][ii] = rhoV
		qD[3][ii] = E
	}
	return
}