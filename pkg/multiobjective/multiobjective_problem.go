package multiobjective

import (
	"log"
	"math"
	"math/rand/v2"
	"strconv"

	v1 "k8s.io/api/core/v1"
	fwk "k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/framework"
)

const (
	ProblemName = "SchedulingProblem"

	Group                   = "multiobjective.x-k8s.io"
	NodeAnnotationPowerIdle = Group + "/power-idle"
	NodeAnnotationPowerBusy = Group + "/power-busy"
)

type SchedulingProblem struct {
	pod       *v1.Pod
	nodeInfos []*fwk.NodeInfo
}

func NewSchedulingProblem(pod *v1.Pod, nodeInfos []*fwk.NodeInfo) *SchedulingProblem {
	return &SchedulingProblem{
		pod:       pod,
		nodeInfos: nodeInfos,
	}
}

func (p *SchedulingProblem) Name() string {
	return ProblemName
}

func (p *SchedulingProblem) ObjectiveFuncs() []framework.ObjectiveFunc {
	return []framework.ObjectiveFunc{
		p.f1, p.f2,
	}
}

func (p *SchedulingProblem) f1(x framework.Solution) float64 {
	xx := x.(*framework.BinarySolution)

	var totalPowerConsumption float64
	for i, bit := range xx.Bits {
		if i >= len(p.nodeInfos) {
			log.Fatalf("more bits in binary solution (%d) than available nodes (%d)", len(xx.Bits), len(p.nodeInfos))
		}

		if bit {
			totalPowerConsumption = calculatePowerConsumption(p.pod, p.nodeInfos[i])
			break
		}
	}

	return totalPowerConsumption
}

func (p *SchedulingProblem) f2(x framework.Solution) float64 {
	xx := x.(*framework.BinarySolution)

	var spreadingScore float64
	for i, bit := range xx.Bits {
		if i >= len(p.nodeInfos) {
			log.Fatalf("more bits in binary solution (%d) than available nodes (%d)", len(xx.Bits), len(p.nodeInfos))
		}

		if bit {
			spreadingScore = calculateSpreadingScore(p.pod, p.nodeInfos[i])
			break
		}
	}

	return spreadingScore
}

// calculatePowerConsumption implements the power consumption model:
// P = Pidle + (Pbusy - Pidle) × (2u - u^r)
// This is taken from "Energy Aware Resource Management of Cloud Data Centers (2017)",
// but is adjusted to not contain the calibration parameter.
// That means the equation becomes P = Pidle + (Pbusy - Pidle) × u
func calculatePowerConsumption(pod *v1.Pod, node *fwk.NodeInfo) float64 {
	// Get node power characteristics from annotations
	pIdle := getFloatFromAnnotation(node.Node(), NodeAnnotationPowerIdle)
	pBusy := getFloatFromAnnotation(node.Node(), NodeAnnotationPowerBusy)

	// Calculate current CPU utilization
	currentMilliCPU := float64(node.Requested.MilliCPU)
	allocatableMilliCPU := float64(node.Allocatable.MilliCPU)

	// Get pod's CPU request
	podMilliCPU := float64(getPodMilliCPURequest(pod))

	currentUtil := currentMilliCPU / allocatableMilliCPU
	newUtil := (currentMilliCPU + podMilliCPU) / allocatableMilliCPU

	// We use an exponential decay function to penalize low utilization
	// The lower the utilization, the higher the penalty
	utilizationThreshold := 0.2
	var penalty float64
	if currentUtil < utilizationThreshold {
		// Calculate penalty that decreases exponentially as utilization approaches the threshold
		penalty = pIdle * math.Exp(-5.0*currentUtil/utilizationThreshold)
	}
	powerConsumption := pIdle + (pBusy-pIdle)*newUtil + penalty
	return powerConsumption
}

func calculateSpreadingScore(pod *v1.Pod, node *fwk.NodeInfo) float64 {
	requested := node.Requested
	allocatable := node.Allocatable

	// Get pod's resource requests
	podMilliCPU := float64(getPodMilliCPURequest(pod))
	podMemory := float64(getPodMemoryRequest(pod))

	// Calculate spread score based on resource imbalance
	newCPUUtil := (float64(requested.MilliCPU) + podMilliCPU) / float64(allocatable.MilliCPU)
	newMemUtil := (float64(requested.Memory) + podMemory) / float64(allocatable.Memory)

	// Use standard deviation from ideal spread as our score
	// We aim for 50% utilization as ideal spread
	idealUtil := 0.5
	cpuDev := math.Abs(newCPUUtil - idealUtil)
	memDev := math.Abs(newMemUtil - idealUtil)

	// Combine deviations (weighted equally)
	spreadScore := (cpuDev + memDev) / 2.0

	// Add penalty for node pod count to encourage pod spreading
	podCountRatio := float64(len(node.Pods)) / float64(node.Allocatable.AllowedPodNumber)
	spreadScore += podCountRatio

	return spreadScore
}

func getPodMilliCPURequest(pod *v1.Pod) int64 {
	var total int64
	for _, container := range pod.Spec.Containers {
		if cpu := container.Resources.Requests.Cpu(); cpu != nil {
			milliCPU := cpu.MilliValue()

			// Check for potential overflow before adding
			if milliCPU > 0 && total > math.MaxInt64-milliCPU {
				// Handle overflow by capping at MaxInt64
				return math.MaxInt64
			}
			total += milliCPU
		}
	}
	return total
}

func getPodMemoryRequest(pod *v1.Pod) int64 {
	var total int64
	for _, container := range pod.Spec.Containers {
		if mem := container.Resources.Requests.Memory(); mem != nil {
			total += mem.Value()
		}
	}
	return total
}

func getFloatFromAnnotation(node *v1.Node, key string) float64 {
	if value, exists := node.Annotations[key]; exists {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return 0.0
}

func getNodeIndex(sol framework.Solution) int {
	s := sol.(*framework.BinarySolution)
	for i, bit := range s.Bits {
		if bit {
			return i
		}
	}
	return -1
}

func (p *SchedulingProblem) Constraints() []framework.Constraint {
	return []framework.Constraint{
		// Constraint 1:
		// The binary solution should contain 1 bit set at most
		// at a time (meaning we only assign the pod to one node)
		func(s framework.Solution) bool {
			bits := s.(*framework.BinarySolution).Bits
			count := 0

			for _, b := range bits {
				if b {
					count++
				}
			}

			return count == 1
		},
	}
}

func (p *SchedulingProblem) Bounds() []framework.Bounds {
	return nil
}

func (p *SchedulingProblem) Initialize(popSize int) []framework.Solution {
	population := make([]framework.Solution, popSize)

	for i := 0; i < popSize; i++ {
		bits := make([]bool, len(p.nodeInfos))
		idx := rand.IntN(len(p.nodeInfos))
		bits[idx] = true

		sol := framework.NewBinarySolution(bits)
		population[i] = sol
	}

	return population
}

func (p *SchedulingProblem) TrueParetoFront(int) []framework.ObjectiveSpacePoint {
	return nil
}
