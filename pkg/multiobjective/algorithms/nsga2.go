package algorithms

import (
	"fmt"
	"log"
	"math"
	"sort"

	"golang.org/x/exp/rand"
	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/framework"
)

const (
	Name = "NSGA-II"
)

// NSGAIISolution wraps a solution in the population
// with Rank and Distance fields. Value stores the value in
// the objective space for the solution (this is used when comparing
// solutions).
type NSGAIISolution struct {
	Solution framework.Solution
	Value    framework.ObjectiveSpacePoint

	Rank     int
	Distance float64
}

func NewNSGAIISolution(sol framework.Solution, val framework.ObjectiveSpacePoint) *NSGAIISolution {
	return &NSGAIISolution{
		Solution: sol,
		Value:    val,
	}
}

// Normalizer handles objective value normalization
type Normalizer struct {
	min []float64
	max []float64
}

// NewNormalizer creates a normalizer for the given number of objectives
func NewNormalizer(min []float64, max []float64) *Normalizer {
	return &Normalizer{
		min: min,
		max: max,
	}
}

// Normalize returns normalized objective values in [0,1]
func (n *Normalizer) Normalize(values []float64) []float64 {
	normalized := make([]float64, len(values))
	for i, val := range values {
		// Avoid division by zero
		if n.max[i] == n.min[i] {
			normalized[i] = 0
		} else {
			normalized[i] = (val - n.min[i]) / (n.max[i] - n.min[i])
		}
	}
	return normalized
}

// SelectByWeights selects the best solution from the population based on weighted objectives
func SelectByWeights(population []*NSGAIISolution, weights []float64) *NSGAIISolution {
	if len(population) == 0 {
		return nil
	}

	bestScore := math.Inf(1)
	var bestSolution *NSGAIISolution

	for _, sol := range population {
		// Calculate weighted sum of normalized objectives
		// We're using (1 - w) because we don't want a higher weight
		// to impact the objectives (which are minimized).
		score := 0.0
		for i, value := range sol.Value {
			score += value * (1 - weights[i])
		}

		// Update best if we found a better score
		if score < bestScore {
			bestScore = score
			bestSolution = sol
		}
	}

	return bestSolution
}

// NonDominatedSort performs non-dominated sorting on the population
func NonDominatedSort(population []*NSGAIISolution) [][]*NSGAIISolution {
	var fronts [][]*NSGAIISolution
	dominated := make(map[int][]int)
	domCount := make([]int, len(population))

	// Calculate domination for each individual
	for i := 0; i < len(population); i++ {
		dominated[i] = []int{}
		for j := 0; j < len(population); j++ {
			if i != j {
				if Dominates(population[i], population[j]) {
					dominated[i] = append(dominated[i], j)
				} else if Dominates(population[j], population[i]) {
					domCount[i]++
				}
			}
		}
	}

	// Find first front
	currentFront := []*NSGAIISolution{}
	currentFrontIndices := []int{}
	for i := 0; i < len(population); i++ {
		if domCount[i] == 0 {
			population[i].Rank = 0
			currentFront = append(currentFront, population[i])
			currentFrontIndices = append(currentFrontIndices, i)
		}
	}
	fronts = append(fronts, currentFront)

	// Find subsequent fronts
	frontIndex := 0
	for len(currentFront) > 0 {
		nextFront := []*NSGAIISolution{}
		nextFrontIndices := []int{}
		for _, idx := range currentFrontIndices {
			for _, dominatedIdx := range dominated[idx] {
				domCount[dominatedIdx]--
				if domCount[dominatedIdx] == 0 {
					population[dominatedIdx].Rank = frontIndex + 1
					nextFront = append(nextFront, population[dominatedIdx])
					nextFrontIndices = append(nextFrontIndices, dominatedIdx)
				}
			}
		}
		frontIndex++
		if len(nextFront) > 0 {
			fronts = append(fronts, nextFront)
		}
		currentFront = nextFront
		currentFrontIndices = nextFrontIndices
	}

	return fronts
}

// Dominates checks if individual a dominates individual b
func Dominates(a, b *NSGAIISolution) bool {
	better := false
	for i := 0; i < len(a.Value); i++ {
		if a.Value[i] > b.Value[i] {
			return false
		}
		if a.Value[i] < b.Value[i] {
			better = true
		}
	}
	return better
}

// CrowdingDistance calculates crowding distance for individuals in a front
func CrowdingDistance(front []*NSGAIISolution) {
	if len(front) <= 2 {
		for i := range front {
			front[i].Distance = math.Inf(1)
		}
		return
	}

	numObjectives := len(front[0].Value)
	for i := range front {
		front[i].Distance = 0
	}

	for m := 0; m < numObjectives; m++ {
		// Sort by each objective
		sort.Slice(front, func(i, j int) bool {
			return front[i].Value[m] < front[j].Value[m]
		})

		// Set boundary points to infinity
		front[0].Distance = math.Inf(1)
		front[len(front)-1].Distance = math.Inf(1)

		objectiveRange := front[len(front)-1].Value[m] - front[0].Value[m]
		if objectiveRange == 0 {
			continue
		}

		// Calculate distance for intermediate points
		for i := 1; i < len(front)-1; i++ {
			front[i].Distance += (front[i+1].Value[m] - front[i-1].Value[m]) / objectiveRange
		}
	}
}

// Tournament selection
func TournamentSelect(population []*NSGAIISolution) *NSGAIISolution {
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

// NSGAII represents the NSGA-II algorithm configuration
type NSGAII struct {
	PopSize        int
	NumGenerations int
	Problem        framework.Problem
	CrossoverRate  float64
	MutationRate   float64
}

// NewNSGAII creates a new instance of NSGA-II with given parameters
func NewNSGAII(popSize, numGen int, problem framework.Problem) *NSGAII {
	return &NSGAII{
		PopSize:        popSize,
		NumGenerations: numGen,
		Problem:        problem,
		CrossoverRate:  0.8,
		MutationRate:   0.1,
	}
}

// Evaluate evaluates the constraints and calculates objective values for an individual
func (n *NSGAII) Evaluate(individual framework.Solution) (framework.ObjectiveSpacePoint, error) {
	constraints := n.Problem.Constraints()
	for _, c := range constraints {
		if !c(individual) {
			return nil, fmt.Errorf("constraint %v failed on this solution", c)
		}
	}

	objectives := n.Problem.ObjectiveFuncs()
	res := make([]float64, len(objectives))

	for i, objFunc := range objectives {
		res[i] = objFunc(individual)
	}
	return res, nil
}

// Run executes the NSGA-II algorithm
func (n *NSGAII) Run() []*NSGAIISolution {
	initPop := n.Problem.Initialize(n.PopSize)
	if len(initPop) != n.PopSize {
		log.Fatalf("could not initialize population with PopSize %d", n.PopSize)
	}

	population := make([]*NSGAIISolution, n.PopSize)
	for i := range n.PopSize {
		val, err := n.Evaluate(initPop[i])
		if err != nil {
			log.Fatalf("evaluate error: %v", err)
		}
		population[i] = NewNSGAIISolution(initPop[i], val)
	}

	for gen := 0; gen < n.NumGenerations; gen++ {
		offspring := make([]*NSGAIISolution, n.PopSize)

		// Generate offspring
		for i := 0; i < n.PopSize; i += 2 {
			parent1 := TournamentSelect(population)
			parent2 := TournamentSelect(population)

			child1, child2 := parent1.Solution.Crossover(parent2.Solution, n.CrossoverRate)
			child1.Mutate(n.MutationRate)
			child2.Mutate(n.MutationRate)

			val1, err := n.Evaluate(child1)
			if err != nil {
				offspring[i] = NewNSGAIISolution(parent1.Solution.Clone(), parent1.Value)
			} else {
				offspring[i] = NewNSGAIISolution(child1, val1)
			}

			if i+1 >= n.PopSize {
				break
			}

			val2, err := n.Evaluate(child2)
			if err != nil {
				offspring[i+1] = NewNSGAIISolution(parent2.Solution.Clone(), parent2.Value)
			} else {
				offspring[i+1] = NewNSGAIISolution(child2, val2)
			}
		}

		// Combine populations
		combined := append(population, offspring...)

		// Non-dominated sorting
		fronts := NonDominatedSort(combined)

		// Clear population for next generation
		population = make([]*NSGAIISolution, 0, n.PopSize)
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
