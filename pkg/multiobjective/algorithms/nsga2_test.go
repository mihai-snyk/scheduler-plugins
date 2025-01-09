package algorithms

import (
	"testing"

	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/benchmarks"
	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/framework"
	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/util"
)

// Test problem: ZDT1 benchmark function
func TestNSGAIIWithZDT1(t *testing.T) {
	numVars := 30
	zdt1 := benchmarks.NewZDT1(numVars)
	varMin := zdt1.LowerBounds()
	varMax := zdt1.UpperBounds()
	objectives := zdt1.ObjectiveFuncs()

	// Create NSGA-II instance
	nsga := NewNSGAII(100, 250, numVars, varMin, varMax, objectives)

	// Run algorithm
	finalPop := nsga.Run()

	// Basic validation
	if len(finalPop) != nsga.PopSize {
		t.Errorf("Expected population size %d, got %d", nsga.PopSize, len(finalPop))
	}

	// Check if solutions are within bounds
	for _, ind := range finalPop {
		for i, val := range ind.Variables {
			if val < varMin[i] || val > varMax[i] {
				t.Errorf("Solution outside bounds: %v", val)
			}
		}
	}

	// Verify Pareto front characteristics
	fronts := framework.NonDominatedSort(finalPop)
	if len(fronts) == 0 {
		t.Error("No fronts found in final population")
	}

	firstFront := fronts[0]
	err := util.PlotResults(zdt1, firstFront, Name)
	if err != nil {
		t.Errorf("Plot failed: %v", err)
	}

	// Check if first front is non-dominated
	for i := 0; i < len(firstFront); i++ {
		for j := 0; j < len(firstFront); j++ {
			if i != j && framework.Dominates(firstFront[i], firstFront[j]) {
				t.Error("First front contains dominated solutions")
			}
		}
	}

	// Maybe check if the first front is "close" to the
	// true Pareto optimal set
	// TODO
}
