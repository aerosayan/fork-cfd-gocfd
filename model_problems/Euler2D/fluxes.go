package Euler2D

import (
	"fmt"
	"math"
	"strings"

	"github.com/notargets/gocfd/utils"
)

func (it InitType) Print() (txt string) {
	txt = InitPrintNames[it]
	return
}

type FluxType uint

const (
	FLUX_Average FluxType = iota
	FLUX_LaxFriedrichs
	FLUX_Roe
)

var (
	FluxNames = map[string]FluxType{
		"average": FLUX_Average,
		"lax":     FLUX_LaxFriedrichs,
		"roe":     FLUX_Roe,
	}
	FluxPrintNames = []string{"Average", "Lax Friedrichs", "Roe"}
)

func (ft FluxType) Print() (txt string) {
	txt = FluxPrintNames[ft]
	return
}

func NewFluxType(label string) (ft FluxType) {
	var (
		ok  bool
		err error
	)
	label = strings.ToLower(label)
	if ft, ok = FluxNames[label]; !ok {
		err = fmt.Errorf("unable to use flux named %s", label)
		panic(err)
	}
	return
}

func (c *Euler) CalculateFluxTransformed(k, Kmax, i int, Jdet, Jinv utils.Matrix, Q [4]utils.Matrix) (Fr, Fs [4]float64) {
	var (
		JdetD = Jdet.DataP[k]
		JinvD = Jinv.DataP[4*k : 4*(k+1)]
	)
	ind := k + i*Kmax
	Fx, Fy := c.CalculateFlux(Q, ind)
	for n := 0; n < 4; n++ {
		Fr[n] = JdetD * (JinvD[0]*Fx[n] + JinvD[1]*Fy[n])
		Fs[n] = JdetD * (JinvD[2]*Fx[n] + JinvD[3]*Fy[n])
	}
	return
}

func (c *Euler) CalculateFlux(QQ [4]utils.Matrix, ind int) (Fx, Fy [4]float64) {
	Fx, Fy = c.FluxCalcMock(QQ[0].DataP[ind], QQ[1].DataP[ind], QQ[2].DataP[ind], QQ[3].DataP[ind])
	return
}

func (c *Euler) FluxCalcBase(rho, rhoU, rhoV, E float64) (Fx, Fy [4]float64) {
	var (
		oorho = 1. / rho
		u     = rhoU * oorho
		v     = rhoV * oorho
		p     = c.FS.GetFlowFunctionBase(rho, rhoU, rhoV, E, StaticPressure)
	)
	Fx, Fy =
		[4]float64{rhoU, rhoU*u + p, rhoU * v, u * (E + p)},
		[4]float64{rhoV, rhoV * u, rhoV*v + p, v * (E + p)}
	return
}

func (c *Euler) FluxJacobianCalcBase(rho, rhoU, rhoV, E float64, F, G utils.Matrix) {
	var (
		oorho  = 1. / rho
		u, v   = rhoU * oorho, rhoV * oorho
		u2, v2 = u * u, v * v
		Gamma  = c.FS.Gamma
		GM1    = Gamma - 1
	)
	F.Equate([]float64{
		0, 1, 0, 0,
		0.5*GM1*(u2+v2) - u2, u * (3. - Gamma), -v * GM1, GM1,
		-u * v, v, u, 0,
		u * ((u2+v2)*GM1 - E*Gamma*oorho), E*Gamma*oorho - 0.5*GM1*(3.*u2+v2), -u * v * GM1, Gamma * u,
	}, ":", ":")
	G.Equate([]float64{
		0, 0, 1, 0,
		-u * v, v, u, 0,
		0.5*GM1*(u2+v2) - v2, -u * GM1, v * (3. - Gamma), GM1,
		v * ((u2+v2)*GM1 - E*Gamma*oorho), -u * v * GM1, E*Gamma*oorho - 0.5*GM1*(3.*v2+u2), Gamma * v,
	}, ":", ":")

}

func (c *Euler) AvgFlux(kL, kR, KmaxL, KmaxR, shiftL, shiftR int,
	Q_FaceL, Q_FaceR [4]utils.Matrix, normal [2]float64, normalFlux, normalFluxReversed [][4]float64) {
	var (
		Nedge = c.dfr.FluxElement.Nedge
	)
	averageFluxN := func(fx1, fy1, fx2, fy2 [4]float64, normal [2]float64) (fnorm [4]float64, fnormR [4]float64) {
		var (
			fave [2][4]float64
		)
		for n := 0; n < 4; n++ {
			fave[0][n] = 0.5 * (fx1[n] + fx2[n])
			fave[1][n] = 0.5 * (fy1[n] + fy2[n])
			fnorm[n] = normal[0]*fave[0][n] + normal[1]*fave[1][n]
			fnormR[n] = -fnorm[n]
		}
		return
	}
	for i := 0; i < Nedge; i++ {
		iL := i + shiftL
		iR := Nedge - 1 - i + shiftR // Shared edges run in reverse order relative to each other
		indL, indR := kL+iL*KmaxL, kR+iR*KmaxR
		FxL, FyL := c.CalculateFlux(Q_FaceL, indL)
		FxR, FyR := c.CalculateFlux(Q_FaceR, indR) // Reverse the right edge to match
		normalFlux[i], normalFluxReversed[Nedge-1-i] = averageFluxN(FxL, FyL, FxR, FyR, normal)
	}
}

func (c *Euler) LaxFlux(kL, kR, KmaxL, KmaxR, shiftL, shiftR int,
	Q_FaceL, Q_FaceR [4]utils.Matrix, normal [2]float64, normalFlux, normalFluxReversed [][4]float64) {
	var (
		Nedge          = c.dfr.FluxElement.Nedge
		uL, vL, pL, CL float64
		uR, vR, pR, CR float64
	)
	for i := 0; i < Nedge; i++ {
		iL := i + shiftL
		iR := Nedge - 1 - i + shiftR // Shared edges run in reverse order relative to each other
		indL, indR := kL+iL*KmaxL, kR+iR*KmaxR
		uL, vL = Q_FaceL[1].DataP[indL]/Q_FaceL[0].DataP[indL], Q_FaceL[2].DataP[indL]/Q_FaceL[0].DataP[indL]
		uR, vR = Q_FaceR[1].DataP[indR]/Q_FaceR[0].DataP[indR], Q_FaceR[2].DataP[indR]/Q_FaceR[0].DataP[indR]
		pL, pR = c.FS.GetFlowFunction(Q_FaceL, indL, StaticPressure), c.FS.GetFlowFunction(Q_FaceR, indR, StaticPressure)
		CL, CR = c.FS.GetFlowFunction(Q_FaceL, indL, SoundSpeed), c.FS.GetFlowFunction(Q_FaceR, indR, SoundSpeed)
		maxV := math.Max(math.Sqrt(uL*uL+vL*vL)+CL, math.Sqrt(uR*uR+vR*vR)+CR)
		normalFlux[i][0] = 0.5 * (normal[0]*(Q_FaceL[1].DataP[indL]+Q_FaceR[1].DataP[indR]) +
			normal[1]*(Q_FaceL[2].DataP[indL]+Q_FaceR[2].DataP[indR]))
		normalFlux[i][1] = 0.5 * (normal[0]*(Q_FaceL[1].DataP[indL]*uL+Q_FaceR[1].DataP[indR]*uR+pL+pR) +
			normal[1]*(Q_FaceL[1].DataP[indL]*vL+Q_FaceR[1].DataP[indR]*vR))
		normalFlux[i][2] = 0.5 * (normal[0]*(Q_FaceL[2].DataP[indL]*uL+Q_FaceR[2].DataP[indR]*uR) +
			normal[1]*(Q_FaceL[2].DataP[indL]*vL+Q_FaceR[2].DataP[indR]*vR+pL+pR))
		normalFlux[i][3] = 0.5 * (normal[0]*((pL+Q_FaceL[3].DataP[indL])*uL+(pR+Q_FaceR[3].DataP[indR])*uR) +
			normal[1]*((pL+Q_FaceL[3].DataP[indL])*vL+(pR+Q_FaceR[3].DataP[indR])*vR))
		for n := 0; n < 4; n++ {
			normalFlux[i][n] += 0.5 * maxV * (Q_FaceL[n].DataP[indL] - Q_FaceR[n].DataP[indR])
			normalFluxReversed[Nedge-1-i][n] = -normalFlux[i][n]
		}
	}
}

func (c *Euler) RoeFlux(kL, kR, KmaxL, KmaxR, shiftL, shiftR int,
	Q_FaceL, Q_FaceR [4]utils.Matrix, normal [2]float64, normalFlux, normalFluxReversed [][4]float64) {
	var (
		Nedge            = c.dfr.FluxElement.Nedge
		rhoL, uL, vL, pL float64
		rhoR, uR, vR, pR float64
		hL, hR           float64
		Gamma            = c.FS.Gamma
		GM1              = Gamma - 1
	)
	for i := 0; i < Nedge; i++ {
		iL := i + shiftL
		iR := Nedge - 1 - i + shiftR // Shared edges run in reverse order relative to each other
		indL, indR := kL+iL*KmaxL, kR+iR*KmaxR
		// Rotate the momentum into face normal coordinates before calculating fluxes
		Q_FaceL[1].DataP[indL], Q_FaceL[2].DataP[indL] = Q_FaceL[1].DataP[indL]*normal[0]+Q_FaceL[2].DataP[indL]*normal[1],
			-Q_FaceL[1].DataP[indL]*normal[1]+Q_FaceL[2].DataP[indL]*normal[0]
		Q_FaceR[1].DataP[indR], Q_FaceR[2].DataP[indR] = Q_FaceR[1].DataP[indR]*normal[0]+Q_FaceR[2].DataP[indR]*normal[1],
			-Q_FaceR[1].DataP[indR]*normal[1]+Q_FaceR[2].DataP[indR]*normal[0]
		rhoL, uL, vL = Q_FaceL[0].DataP[indL], Q_FaceL[1].DataP[indL]/Q_FaceL[0].DataP[indL], Q_FaceL[2].DataP[indL]/Q_FaceL[0].DataP[indL]
		rhoR, uR, vR = Q_FaceR[0].DataP[indR], Q_FaceR[1].DataP[indR]/Q_FaceR[0].DataP[indR], Q_FaceR[2].DataP[indR]/Q_FaceR[0].DataP[indR]
		pL, pR = c.FS.GetFlowFunction(Q_FaceL, indL, StaticPressure), c.FS.GetFlowFunction(Q_FaceR, indR, StaticPressure)
		/*
		   HM = (EnerM+pM).dd(rhoM);  HP = (EnerP+pP).dd(rhoP);
		*/
		// Enthalpy
		hL, hR = (Q_FaceL[3].DataP[indL]+pL)/rhoL, (Q_FaceR[3].DataP[indR]+pR)/rhoR
		/*
			rhoMs = sqrt(rhoM); rhoPs = sqrt(rhoP);
			rhoMsPs = rhoMs + rhoPs;

			rho = rhoMs.dm(rhoPs);
			u   = (rhoMs.dm(uM) + rhoPs.dm(uP)).dd(rhoMsPs);
			v   = (rhoMs.dm(vM) + rhoPs.dm(vP)).dd(rhoMsPs);
			H   = (rhoMs.dm(HM) + rhoPs.dm(HP)).dd(rhoMsPs);
			c2 = gm1 * (H - 0.5*(sqr(u)+sqr(v)));
			c = sqrt(c2);
		*/
		// Compute Roe average variables
		rhoLs, rhoRs := math.Sqrt(rhoL), math.Sqrt(rhoR)
		rhoLsRs := rhoLs + rhoRs

		rho := rhoLs * rhoRs
		u := (rhoLs*uL + rhoRs*uR) / rhoLsRs
		v := (rhoLs*vL + rhoRs*vR) / rhoLsRs
		h := (rhoLs*hL + rhoRs*hR) / rhoLsRs
		c2 := GM1 * (h - 0.5*(u*u+v*v))
		c := math.Sqrt(c2)
		/*
		   dW1 = -0.5*(rho.dm(uP-uM)).dd(c) + 0.5*(pP-pM).dd(c2);
		   dW2 = (rhoP-rhoM) - (pP-pM).dd(c2);
		   dW3 = rho.dm(vP-vM);
		   dW4 = 0.5*(rho.dm(uP-uM)).dd(c) + 0.5*(pP-pM).dd(c2);

		   dW1 = abs(u-c).dm(dW1);
		   dW2 = abs(u  ).dm(dW2);
		   dW3 = abs(u  ).dm(dW3);
		   dW4 = abs(u+c).dm(dW4);
		*/
		// Riemann fluxes
		dW1 := -0.5*(rho*(uR-uL))/c + 0.5*(pR-pL)/c2
		dW2 := (rhoR - rhoL) - (pR-pL)/c2
		dW3 := rho * (vR - vL)
		dW4 := 0.5*(rho*(uR-uL))/c + 0.5*(pR-pL)/c2
		dW1 = math.Abs(u-c) * dW1
		dW2 = math.Abs(u) * dW2
		dW3 = math.Abs(u) * dW3
		dW4 = math.Abs(u+c) * dW4
		/*
		   DMat fx = (fxQP+fxQM)/2.0;
		   fx(All,1) -= (dW1               + dW2                                   + dW4              )/2.0;
		   fx(All,2) -= (dW1.dm(u-c)       + dW2.dm(u)                             + dW4.dm(u+c)      )/2.0;
		   fx(All,3) -= (dW1.dm(v)         + dW2.dm(v)                 + dW3       + dW4.dm(v)        )/2.0;
		   fx(All,4) -= (dW1.dm(H-u.dm(c)) + dW2.dm(sqr(u)+sqr(v))/2.0 + dW3.dm(v) + dW4.dm(H+u.dm(c)))/2.0;
		*/
		// Form Roe Fluxes
		// Ave of normal component of flux
		normalFlux[i][0] = 0.5 * (Q_FaceL[1].DataP[indL] + Q_FaceR[1].DataP[indR])
		normalFlux[i][1] = 0.5 * (Q_FaceL[1].DataP[indL]*uL + Q_FaceR[1].DataP[indR]*uR + +pL + pR)
		normalFlux[i][2] = 0.5 * (Q_FaceL[2].DataP[indL]*uL + Q_FaceR[2].DataP[indR]*uR)
		normalFlux[i][3] = 0.5 * ((pL+Q_FaceL[3].DataP[indL])*uL + (pR+Q_FaceR[3].DataP[indR])*uR)

		normalFlux[i][0] -= 0.5 * (dW1 + dW2 + dW4)
		normalFlux[i][1] -= 0.5 * (dW1*(u-c) + dW2*u + dW4*(u+c))
		normalFlux[i][2] -= 0.5 * (dW1*v + dW2*v + dW3 + dW4*v)
		normalFlux[i][3] -= 0.5 * (dW1*(h-u*c) + 0.5*dW2*(u*u+v*v) + dW3*v + dW4*(h+u*c))
		/*
		   flux = fx;    fx2.borrow(Ngf, fx.pCol(2)); fx3.borrow(Ngf, fx.pCol(3));
		   flux(All,2) = lnx.dm(fx2) - lny.dm(fx3);
		   flux(All,3) = lny.dm(fx2) + lnx.dm(fx3);
		*/
		// rotate back to Cartesian
		normalFlux[i][1], normalFlux[i][2] = normal[0]*normalFlux[i][1]-normal[1]*normalFlux[i][2],
			normal[1]*normalFlux[i][1]+normal[0]*normalFlux[i][2]
		for n := 0; n < 4; n++ {
			normalFluxReversed[Nedge-1-i][n] = -normalFlux[i][n]
		}
	}
}
