package framework

// NonDominatedSort performs non-dominated sorting on the population
func NonDominatedSort(population []Individual) [][]Individual {
	var fronts [][]Individual
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
	currentFront := []Individual{}
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
		nextFront := []Individual{}
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
func Dominates(a, b Individual) bool {
	better := false
	for i := 0; i < len(a.Objectives); i++ {
		if a.Objectives[i] > b.Objectives[i] {
			return false
		}
		if a.Objectives[i] < b.Objectives[i] {
			better = true
		}
	}
	return better
}
