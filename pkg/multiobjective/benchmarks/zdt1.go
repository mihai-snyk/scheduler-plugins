package benchmarks

import (
	"math"

	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/framework"
)

const (
	Name = "ZDT1"
)

// ZDT1 is a benchmark function used to test the correctness
// of multi-objective algorithms. For more details, check the article below:
// https://datacrayon.com/practical-evolutionary-algorithms/synthetic-objective-functions-and-zdt1/
type ZDT1 struct {
	numVars int
}

func NewZDT1(numVars int) *ZDT1 {
	return &ZDT1{
		numVars,
	}
}

func (p *ZDT1) Name() string {
	return Name
}

func (p *ZDT1) ObjectiveFuncs() []framework.ObjectiveFunc {
	return []framework.ObjectiveFunc{
		p.f1, p.f2,
	}
}

// F1 is the first ZD1 benchmark objective
func (p *ZDT1) f1(x []float64) float64 {
	return x[0]
}

// F2 is the first ZD1 benchmark objective
func (p *ZDT1) f2(x []float64) float64 {
	g := 1.0
	for i := 1; i < len(x); i++ {
		g += 9.0 * x[i] / float64(len(x)-1)
	}
	return g * (1.0 - math.Sqrt(x[0]/g))
}

func (p *ZDT1) LowerBounds() []float64 {
	varMin := make([]float64, p.numVars)
	for i := range p.numVars {
		varMin[i] = 0.0
	}
	return varMin
}

func (p *ZDT1) UpperBounds() []float64 {
	varMax := make([]float64, p.numVars)
	for i := range p.numVars {
		varMax[i] = 1.0
	}
	return varMax
}

// TrueParetoFront generates numPoints points on the true Pareto front for ZDT1
func (p *ZDT1) TrueParetoFront(numPoints int) []framework.ObjectiveSpacePoint {
	points := make([]framework.ObjectiveSpacePoint, numPoints)
	for i := 0; i < numPoints; i++ {
		x := float64(i) / float64(numPoints-1)
		points[i] = framework.ObjectiveSpacePoint{
			x, 1.0 - math.Sqrt(x),
		}
	}
	return points
}
