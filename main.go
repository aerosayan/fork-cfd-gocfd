package main

import (
	"flag"
	"fmt"
	"math"
	"time"

	"gonum.org/v1/gonum/mat"

	"github.com/notargets/gocfd/model_problems"

	"github.com/notargets/gocfd/DG1D"
)

var (
	K     = 10 // Number of elements
	N     = 1  // Polynomial degree
	Delay = time.Duration(200)
)

func main() {
	Kptr := flag.Int("K", 10, "Number of elements in model")
	Nptr := flag.Int("N", 4, "polynomial degree")
	Delayptr := flag.Int("delay", 200, "milliseconds of delay for plotting")
	Graphptr := flag.Bool("graph", false, "display a graph while computing solution")
	flag.Parse()
	K = *Kptr
	N = *Nptr
	Delay = time.Duration(*Delayptr)

	VX, EToV := DG1D.SimpleMesh1D(0, 2*math.Pi, K)
	e1D := DG1D.NewElements1D(N, VX, EToV)
	c := model_problems.NewAdvection1D(2*math.Pi, 0.75, 100000., e1D)
	c.Run(*Graphptr, Delay*time.Millisecond)
	fmt.Printf("X = \n%v\n", mat.Formatted(e1D.X, mat.Squeeze()))
}
