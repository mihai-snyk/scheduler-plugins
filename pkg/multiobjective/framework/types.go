package framework

// Individual represents a solution in the population
type Individual struct {
	Variables  []float64
	Objectives []float64

	// Rank is NSGA-II specific
	// TODO: Remove it from the framework
	Rank int
	// Distance is NSGA-II specific
	// TODO: Remove it from the framework
	Distance float64
}

// ObjectiveFunc defines the interface for objective functions
type ObjectiveFunc func([]float64) float64

// ObjectiveSpacePoint represents an N-dimensional point in the objective space.
// As an example, for a problem with 2 objective functions f1 and f2, a point
// in the objective space could be [f1(x'), f2(x')], for the input of x'.
type ObjectiveSpacePoint []float64

// Problem describes the contract a specific multi-objective problem needs to implement.
// TODO: Add more comments for each of the methods
type Problem interface {
	Name() string

	LowerBounds() []float64
	UpperBounds() []float64

	ObjectiveFuncs() []ObjectiveFunc
	TrueParetoFront(int) []ObjectiveSpacePoint
}

// Algorithm describes the contract that a MOO algorithm needs to implement.
// TODO: Improve the abstraction by adding more methods
type Algorithm interface {
	Name() string
}
