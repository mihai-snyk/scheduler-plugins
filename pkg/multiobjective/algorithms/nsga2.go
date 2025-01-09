package algorithms

import (
	"math"
	"math/rand"
	"sort"

	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/framework"
)

const (
	Name = "NSGA-II"
)

// NSGAII represents the NSGA-II algorithm configuration
type NSGAII struct {
	PopSize        int
	NumGenerations int
	NumVariables   int
	VarMin         []float64
	VarMax         []float64
	ObjectiveFuncs []framework.ObjectiveFunc
	CrossoverRate  float64
	MutationRate   float64
}

// NewNSGAII creates a new instance of NSGA-II with given parameters
func NewNSGAII(popSize, numGen, numVar int, varMin, varMax []float64, objFuncs []framework.ObjectiveFunc) *NSGAII {
	return &NSGAII{
		PopSize:        popSize,
		NumGenerations: numGen,
		NumVariables:   numVar,
		VarMin:         varMin,
		VarMax:         varMax,
		ObjectiveFuncs: objFuncs,
		CrossoverRate:  0.8,
		MutationRate:   0.1,
	}
}

// Initialize creates an initial random population of individuals
// TODO: Check if there are better ways to generate scattered solutions
func (n *NSGAII) Initialize() []framework.Individual {
	population := make([]framework.Individual, n.PopSize)

	for i := 0; i < n.PopSize; i++ {
		vars := make([]float64, n.NumVariables)
		for j := 0; j < n.NumVariables; j++ {
			vars[j] = n.VarMin[j] + rand.Float64()*(n.VarMax[j]-n.VarMin[j])
		}

		objs := make([]float64, len(n.ObjectiveFuncs))
		for j, objFunc := range n.ObjectiveFuncs {
			objs[j] = objFunc(vars)
		}

		population[i] = framework.Individual{
			Variables:  vars,
			Objectives: objs,
		}
	}

	return population
}

// CrowdingDistance calculates crowding distance for individuals in a front
func CrowdingDistance(front []framework.Individual) {
	if len(front) <= 2 {
		for i := range front {
			front[i].Distance = math.Inf(1)
		}
		return
	}

	numObjectives := len(front[0].Objectives)
	for i := range front {
		front[i].Distance = 0
	}

	for m := 0; m < numObjectives; m++ {
		// Sort by each objective
		sort.Slice(front, func(i, j int) bool {
			return front[i].Objectives[m] < front[j].Objectives[m]
		})

		// Set boundary points to infinity
		front[0].Distance = math.Inf(1)
		front[len(front)-1].Distance = math.Inf(1)

		objectiveRange := front[len(front)-1].Objectives[m] - front[0].Objectives[m]
		if objectiveRange == 0 {
			continue
		}

		// Calculate distance for intermediate points
		for i := 1; i < len(front)-1; i++ {
			front[i].Distance += (front[i+1].Objectives[m] - front[i-1].Objectives[m]) / objectiveRange
		}
	}
}

// Tournament selection
func (n *NSGAII) TournamentSelect(population []framework.Individual) framework.Individual {
	k := 2 // tournament size
	best := population[rand.Intn(len(population))]

	for i := 1; i < k; i++ {
		contestant := population[rand.Intn(len(population))]
		if contestant.Rank < best.Rank || (contestant.Rank == best.Rank && contestant.Distance > best.Distance) {
			best = contestant
		}
	}

	return best
}

// Crossover performs SBX (Simulated Binary Crossover)
func (n *NSGAII) Crossover(parent1, parent2 framework.Individual) (framework.Individual, framework.Individual) {
	child1 := framework.Individual{Variables: make([]float64, len(parent1.Variables))}
	child2 := framework.Individual{Variables: make([]float64, len(parent2.Variables))}

	if rand.Float64() < n.CrossoverRate {
		for i := range parent1.Variables {
			beta := 0.0
			if rand.Float64() <= 0.5 {
				beta = math.Pow(2*rand.Float64(), 1.0/3.0)
			} else {
				beta = math.Pow(1.0/(2*(1.0-rand.Float64())), 1.0/3.0)
			}

			child1.Variables[i] = 0.5 * ((1+beta)*parent1.Variables[i] + (1-beta)*parent2.Variables[i])
			child2.Variables[i] = 0.5 * ((1-beta)*parent1.Variables[i] + (1+beta)*parent2.Variables[i])

			// Bound checking
			child1.Variables[i] = math.Max(n.VarMin[i], math.Min(n.VarMax[i], child1.Variables[i]))
			child2.Variables[i] = math.Max(n.VarMin[i], math.Min(n.VarMax[i], child2.Variables[i]))
		}
	} else {
		copy(child1.Variables, parent1.Variables)
		copy(child2.Variables, parent2.Variables)
	}

	return child1, child2
}

// Mutation performs polynomial mutation
func (n *NSGAII) Mutation(individual *framework.Individual) {
	for i := range individual.Variables {
		if rand.Float64() < n.MutationRate {
			delta := 0.0
			if rand.Float64() <= 0.5 {
				delta = math.Pow(2*rand.Float64(), 1.0/3.0) - 1
			} else {
				delta = 1 - math.Pow(2*(1-rand.Float64()), 1.0/3.0)
			}

			individual.Variables[i] += delta * (n.VarMax[i] - n.VarMin[i])
			individual.Variables[i] = math.Max(n.VarMin[i], math.Min(n.VarMax[i], individual.Variables[i]))
		}
	}
}

// Evaluate calculates objective values for an individual
func (n *NSGAII) Evaluate(individual *framework.Individual) {
	individual.Objectives = make([]float64, len(n.ObjectiveFuncs))
	for i, objFunc := range n.ObjectiveFuncs {
		individual.Objectives[i] = objFunc(individual.Variables)
	}
}

// Run executes the NSGA-II algorithm
func (n *NSGAII) Run() []framework.Individual {
	population := n.Initialize()

	for gen := 0; gen < n.NumGenerations; gen++ {
		offspring := make([]framework.Individual, n.PopSize)

		// Generate offspring
		for i := 0; i < n.PopSize; i += 2 {
			parent1 := n.TournamentSelect(population)
			parent2 := n.TournamentSelect(population)

			child1, child2 := n.Crossover(parent1, parent2)

			n.Mutation(&child1)
			n.Mutation(&child2)

			n.Evaluate(&child1)
			n.Evaluate(&child2)

			offspring[i] = child1
			if i+1 < n.PopSize {
				offspring[i+1] = child2
			}
		}

		// Combine populations
		combined := append(population, offspring...)

		// Non-dominated sorting
		fronts := framework.NonDominatedSort(combined)

		// Clear population for next generation
		population = make([]framework.Individual, 0, n.PopSize)
		frontIndex := 0

		// Add fronts to new population
		for len(population)+len(fronts[frontIndex]) <= n.PopSize {
			CrowdingDistance(fronts[frontIndex])
			population = append(population, fronts[frontIndex]...)
			frontIndex++
			if frontIndex >= len(fronts) {
				break
			}
		}

		// If needed, add remaining individuals based on crowding distance
		if len(population) < n.PopSize && frontIndex < len(fronts) {
			CrowdingDistance(fronts[frontIndex])
			sort.Slice(fronts[frontIndex], func(i, j int) bool {
				return fronts[frontIndex][i].Distance > fronts[frontIndex][j].Distance
			})
			population = append(population, fronts[frontIndex][:n.PopSize-len(population)]...)
		}
	}

	return population
}
