package multiobjective

import "sigs.k8s.io/scheduler-plugins/pkg/multiobjective/framework"

const (
	ProblemName = "SchedulingProblem"
)

type SchedulingProblem struct {
	numVars int
}

func (p *SchedulingProblem) Name() string {
	return ProblemName
}

func (p *SchedulingProblem) LowerBounds() []float64 {
	return nil
}

func (p *SchedulingProblem) UpperBounds() []float64 {
	return nil
}

func (p *SchedulingProblem) ObjectiveFuncs() []framework.ObjectiveFunc {
	return nil
}

func (p *SchedulingProblem) TrueParetoFront(int) []framework.ObjectiveSpacePoint {
	return nil
}

func (p *SchedulingProblem) f1(x []float64) float64 {
	return 0
}
