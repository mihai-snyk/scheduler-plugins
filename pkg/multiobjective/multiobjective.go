package multiobjective

import (
	"context"
	"fmt"
	"math"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/scheduler-plugins/apis/config"
	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/algorithms"
	fwk "sigs.k8s.io/scheduler-plugins/pkg/multiobjective/framework"
)

type MultiObjective struct {
	handle           framework.Handle
	objectiveWeights []float64
}

var _ framework.PreScorePlugin = &MultiObjective{}
var _ framework.ScorePlugin = &MultiObjective{}

const (
	Name             = "MultiObjective"
	PreScoreStateKey = "PreScore" + Name
)

type preScoreState struct {
	bestNode string
}

func (s *preScoreState) Clone() framework.StateData {
	return s
}

func New(ctx context.Context, obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	logger := klog.FromContext(ctx)

	args, ok := obj.(*config.MultiObjectiveArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type MultiObjectiveArgs, got %T", obj)
	}
	logger.V(5).Info(fmt.Sprintf("plugin MultiObjective called with args %v", args.ObjectiveWeights))

	plugin := &MultiObjective{
		handle:           handle,
		objectiveWeights: args.ObjectiveWeights,
	}

	return plugin, nil
}

func (p *MultiObjective) Name() string {
	return Name
}

func validateWeights(weights []float64, numObjectives int) bool {
	// Validate we have same number of weights as objectives
	if len(weights) != numObjectives {
		return false
	}

	// Validate each weight is in [0,1]
	for _, w := range weights {
		if w < 0 || w > 1 {
			return false
		}
	}

	// Validate weights sum to 1
	sum := 0.0
	for _, w := range weights {
		sum += w
	}

	return math.Abs(sum-1.0) <= 1e-6
}

func (p *MultiObjective) PreScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodes []*framework.NodeInfo) *framework.Status {
	logger := klog.FromContext(ctx)

	problem := NewSchedulingProblem(pod, nodes)
	ok := validateWeights(p.objectiveWeights, len(problem.ObjectiveFuncs()))
	if !ok {
		framework.AsStatus(fmt.Errorf("weights are not set correctly: %v", p.objectiveWeights))
	}

	// TODO: Population size should be configurable
	popSize := 100
	nsga := algorithms.NewNSGAII(popSize, 250, problem)
	finalPop := nsga.Run()

	for _, s := range finalPop {
		sol := s.Solution.(*fwk.BinarySolution)
		for i, bit := range sol.Bits {
			if bit {
				node := nodes[i].Node()
				logger.V(5).Info(fmt.Sprintf("the solution node: %v, rank: %d, value: %v", node.Name, s.Rank, s.Value))
				break
			}
		}
	}

	minf1, maxf1 := findPowerBounds(nodes)
	minf2, maxf2 := 0.0, 3.0

	normalizer := algorithms.NewNormalizer(
		[]float64{minf1, minf2},
		[]float64{maxf1, maxf2},
	)

	for _, sol := range finalPop {
		s := sol.Solution.(*fwk.BinarySolution)
		normalizedValue := normalizer.Normalize(sol.Value)

		originalValue := sol.Value
		sol.Value = normalizedValue

		for i, bit := range s.Bits {
			if bit {
				logger.V(5).Info(fmt.Sprintf("for node %s, the original: %v, normalized value: %v", nodes[i].GetName(), originalValue, normalizedValue))
				break
			}
		}
	}

	finalSol := algorithms.SelectByWeights(finalPop, p.objectiveWeights)
	idx := getNodeIndex(finalSol.Solution)
	logger.V(5).Info(fmt.Sprintf("the final node: %s", nodes[idx].GetName()))

	bestNode := nodes[idx].GetName()
	s := &preScoreState{
		bestNode: bestNode,
	}
	state.Write(PreScoreStateKey, s)

	return framework.NewStatus(framework.Success, "")
}

// Just read from the state and return the scores accordingly.
func (p *MultiObjective) Score(ctx context.Context, state *framework.CycleState, po *v1.Pod, nodeName string) (int64, *framework.Status) {
	c, err := state.Read(PreScoreStateKey)
	if err != nil {
		return 0, framework.AsStatus(fmt.Errorf("reading %q from cycleState: %w", PreScoreStateKey, err))
	}

	s, ok := c.(*preScoreState)
	if !ok {
		return 0, framework.AsStatus(fmt.Errorf("invalid state, got type %T", c))
	}

	if s.bestNode == nodeName {
		return framework.MaxNodeScore, framework.NewStatus(framework.Success, "")
	}

	return framework.MinNodeScore, framework.NewStatus(framework.Success, "")
}

func (p *MultiObjective) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func findPowerBounds(nodes []*framework.NodeInfo) (minPowerIdle, maxPowerTotal float64) {
	if len(nodes) == 0 {
		return 0, 0
	}

	// Initialize with first node's values
	minPowerIdle = getFloatFromAnnotation(nodes[0].Node(), NodeAnnotationPowerIdle)
	pBusy := getFloatFromAnnotation(nodes[0].Node(), NodeAnnotationPowerBusy)
	maxPowerTotal = minPowerIdle + pBusy

	// Check remaining nodes
	for _, node := range nodes[1:] {
		pIdle := getFloatFromAnnotation(node.Node(), NodeAnnotationPowerIdle)
		pBusy := getFloatFromAnnotation(node.Node(), NodeAnnotationPowerBusy)
		powerTotal := pIdle + pBusy

		if pIdle < minPowerIdle {
			minPowerIdle = pIdle
		}
		if powerTotal > maxPowerTotal {
			maxPowerTotal = powerTotal
		}
	}

	return minPowerIdle, maxPowerTotal
}
