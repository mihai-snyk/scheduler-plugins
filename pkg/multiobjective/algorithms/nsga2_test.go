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
	popSize := 100

	// Create the ZDT1 problem instance
	zdt1 := benchmarks.NewZDT1(numVars)

	// Create NSGA-II instance
	nsga := NewNSGAII(popSize, 250, zdt1)

	// Run algorithm
	finalPop := nsga.Run()

	// Basic validation
	if len(finalPop) != nsga.PopSize {
		t.Errorf("Expected population size %d, got %d", nsga.PopSize, len(finalPop))
	}

	// Verify Pareto front characteristics
	fronts := NonDominatedSort(finalPop)
	if len(fronts) == 0 {
		t.Error("No fronts found in final population")
	}

	firstFront := fronts[0]
	results := make([]framework.ObjectiveSpacePoint, len(firstFront))
	for i := range len(firstFront) {
		results[i] = firstFront[i].Value
	}
	err := util.PlotResults(results, zdt1, Name)
	if err != nil {
		t.Errorf("Plot failed: %v", err)
	}

	// Check if first front is non-dominated
	for i := 0; i < len(firstFront); i++ {
		for j := 0; j < len(firstFront); j++ {
			if i != j && Dominates(firstFront[i], firstFront[j]) {
				t.Error("First front contains dominated solutions")
			}
		}
	}
}
