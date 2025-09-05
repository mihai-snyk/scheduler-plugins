/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package multiobjective

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	deschedulerv1alpha1 "sigs.k8s.io/scheduler-plugins/apis/descheduler/v1alpha1"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
)

const (
	Name = "MultiObjective"

	// CycleState key for storing the selected target node
	stateKey = "MultiObjective"

	// Scoring constants
	MinNodeScore = int64(0)   // Minimum score (let NodeResourcesFit take over)
	MaxNodeScore = int64(100) // Maximum score (prefer this node)
)

// MultiObjectiveState stores the selected target node for the current scheduling cycle
type MultiObjectiveState struct {
	TargetNode string                              // The node selected for this pod based on scheduling hints
	HasHint    bool                                // Whether we found a valid scheduling hint
	Hint       *deschedulerv1alpha1.SchedulingHint // The scheduling hint for slot consumption
	RSKey      string                              // The ReplicaSet key for this pod
}

// Clone implements framework.StateData interface
func (m *MultiObjectiveState) Clone() framework.StateData {
	return &MultiObjectiveState{
		TargetNode: m.TargetNode,
		HasHint:    m.HasHint,
		Hint:       m.Hint,
		RSKey:      m.RSKey,
	}
}

// MultiObjectiveScheduler is a scheduler plugin that consumes hints from the descheduler
type MultiObjectiveScheduler struct {
	logger klog.Logger
	handle framework.Handle
}

var _ framework.PreScorePlugin = &MultiObjectiveScheduler{}
var _ framework.ScorePlugin = &MultiObjectiveScheduler{}

// NewScheduler builds the scheduler plugin
func New(ctx context.Context, args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	logger := klog.FromContext(ctx).WithName(Name)

	return &MultiObjectiveScheduler{
		logger: logger,
		handle: handle,
	}, nil
}

// Name returns the plugin name
func (s *MultiObjectiveScheduler) Name() string {
	return Name
}

// PreScore implements the PreScore extension point
func (s *MultiObjectiveScheduler) PreScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, filteredNodes []*framework.NodeInfo) *framework.Status {
	// Get ReplicaSet key for this pod
	rsKey := s.getReplicaSetKey(pod)

	// Initialize state with no hint
	cycleState := &MultiObjectiveState{
		TargetNode: "",
		HasHint:    false,
		Hint:       nil,
		RSKey:      rsKey,
	}
	s.logger.V(4).Info("available nodes beginning", "nodes", len(filteredNodes))

	// Try to get scheduling hint and select target node
	hint, solution, err := s.getSchedulingHint(ctx)
	if err != nil || hint == nil || solution == nil {
		s.logger.V(4).Info("No scheduling hint available - will use default scoring",
			"pod", klog.KObj(pod), "error", err)
		// Store state with no hint - Score will return min scores
		state.Write(stateKey, cycleState)
		return nil
	}

	// Find the best target node for this ReplicaSet from the solution
	targetNode := s.selectBestNode(solution, rsKey, filteredNodes)
	if targetNode != "" {
		cycleState.TargetNode = targetNode
		cycleState.HasHint = true
		cycleState.Hint = hint
		s.logger.V(3).Info("Selected target node from scheduling hint",
			"pod", klog.KObj(pod), "targetNode", targetNode, "replicaSet", rsKey)
	} else {
		s.logger.V(4).Info("No suitable target node found in scheduling hint",
			"pod", klog.KObj(pod), "replicaSet", rsKey)
	}

	// Store state for Score method to use
	state.Write(stateKey, cycleState)
	return nil
}

// Score implements the Score extension point
func (s *MultiObjectiveScheduler) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	// Read the state from PreScore
	data, err := state.Read(stateKey)
	if err != nil {
		s.logger.V(4).Info("Failed to read state - using min score",
			"pod", klog.KObj(pod), "node", nodeName, "error", err)
		return MinNodeScore, nil
	}

	cycleState, ok := data.(*MultiObjectiveState)
	if !ok {
		s.logger.V(4).Info("Invalid state type - using min score",
			"pod", klog.KObj(pod), "node", nodeName)
		return MinNodeScore, nil
	}

	// If we don't have a hint, use min score (let NodeResourcesFit take over)
	if !cycleState.HasHint {
		s.logger.V(4).Info("No scheduling hint available - using min score",
			"pod", klog.KObj(pod), "node", nodeName)
		return MinNodeScore, nil
	}

	// If this is the target node, try to consume a slot atomically
	if nodeName == cycleState.TargetNode {
		// Try to consume a slot for this ReplicaSet on this node
		consumed := s.tryConsumeSlot(ctx, cycleState.Hint, cycleState.RSKey, nodeName)

		if consumed {
			s.logger.V(3).Info("Successfully consumed slot - scoring target node with max score",
				"pod", klog.KObj(pod), "node", nodeName, "replicaSet", cycleState.RSKey, "score", MaxNodeScore)
			return MaxNodeScore, nil
		} else {
			s.logger.V(4).Info("Failed to consume slot on target node - using min score",
				"pod", klog.KObj(pod), "node", nodeName, "replicaSet", cycleState.RSKey, "score", MinNodeScore)
			return MinNodeScore, nil
		}
	}

	// For all other nodes, give min score
	s.logger.V(4).Info("Scoring non-target node with min score",
		"pod", klog.KObj(pod), "node", nodeName, "targetNode", cycleState.TargetNode, "score", MinNodeScore)
	return MinNodeScore, nil
}

// ScoreExtensions returns score extensions (none needed)
func (s *MultiObjectiveScheduler) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// selectBestNode selects the best target node for a ReplicaSet from the scheduling hint solution
func (s *MultiObjectiveScheduler) selectBestNode(solution *deschedulerv1alpha1.OptimizationSolution, rsKey string, filteredNodes []*framework.NodeInfo) string {
	// Create a set of available nodes from filteredNodes
	availableNodes := make(map[string]bool)
	for _, nodeInfo := range filteredNodes {
		if _, isControlPlane := nodeInfo.Node().Labels["node-role.kubernetes.io/control-plane"]; isControlPlane {
			continue
		}
		availableNodes[nodeInfo.Node().Name] = true
	}

	// Find the ReplicaSet movement in the solution
	for _, movement := range solution.ReplicaSetMovements {
		movementKey := fmt.Sprintf("%s/%s", movement.Namespace, movement.ReplicaSetName)
		if movementKey == rsKey {
			// Find the node with the highest target distribution that's also available
			bestNode := ""
			maxTarget := 0

			for nodeName, targetCount := range movement.TargetDistribution {
				s.logger.Info("checking distribution", "node", nodeName, "targetCount", targetCount)
				// Check if this node is in the filtered list (passed scheduling constraints)
				if !availableNodes[nodeName] {
					s.logger.V(4).Info("node not available", "node", nodeName)
					continue
				}

				// Check if this node has available slots
				availableSlots := movement.AvailableSlots[nodeName]
				s.logger.V(4).Info("slots on the node", "node", nodeName, "slots", availableSlots)
				if availableSlots > 0 && targetCount > maxTarget {
					bestNode = nodeName
					maxTarget = targetCount
				}
			}

			s.logger.V(4).Info("Selected best node for ReplicaSet",
				"replicaSet", rsKey, "bestNode", bestNode, "targetCount", maxTarget)
			return bestNode
		}
	}

	s.logger.V(4).Info("No movement found for ReplicaSet in solution", "replicaSet", rsKey)
	return ""
}

// getSchedulingHint fetches the appropriate scheduling hint for a pod
func (s *MultiObjectiveScheduler) getSchedulingHint(ctx context.Context) (*deschedulerv1alpha1.SchedulingHint, *deschedulerv1alpha1.OptimizationSolution, error) {
	// Get cluster state
	nodes, err := s.handle.ClientSet().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Get all ReplicaSets to calculate current cluster fingerprint based on desired state
	replicaSets, err := s.handle.ClientSet().AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list ReplicaSets: %w", err)
	}

	// Calculate current cluster fingerprint based on ReplicaSet desired state
	fingerprint := s.calculateClusterFingerprintFromReplicaSets(ctx, nodes.Items, replicaSets.Items)

	// Get REST config for custom resource client
	config, err := s.getRESTConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get REST config: %w", err)
	}

	// Create clientset
	clientset, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	s.logger.Info("generating the hint name", "fingerprint", fingerprint)
	// Try to get hint for exact cluster fingerprint
	hintName := s.generateHintName(fingerprint)
	hint, err := clientset.DeschedulerV1alpha1().SchedulingHints().Get(ctx, hintName, metav1.GetOptions{})
	if err != nil {
		s.logger.V(4).Info("No scheduling hint found for current cluster state",
			"hintName", hintName, "fingerprint", fingerprint, "error", err.Error())
		return nil, nil, nil // Return nil without error to trigger fallback to default scoring
	}

	// Get the top solution (first one is best)
	if len(hint.Spec.Solutions) == 0 {
		return nil, nil, fmt.Errorf("no solutions in scheduling hint")
	}

	topSolution := &hint.Spec.Solutions[0]

	s.logger.V(3).Info("Found scheduling hint",
		"hint", hint.Name,
		"fingerprint", fingerprint,
		"solutions", len(hint.Spec.Solutions),
		"topSolutionScore", topSolution.WeightedScore,
		"age", time.Since(hint.CreationTimestamp.Time).Round(time.Second))

	return hint, topSolution, nil
}

// calculateClusterFingerprintFromReplicaSets calculates fingerprint based on ReplicaSet desired state
// isSystemNamespace checks if a namespace should be excluded from fingerprint calculation
func isSystemNamespace(namespace string) bool {
	systemNamespaces := []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
		"local-path-storage",
	}

	for _, sysNs := range systemNamespaces {
		if namespace == sysNs {
			return true
		}
	}
	return false
}

func (s *MultiObjectiveScheduler) calculateClusterFingerprintFromReplicaSets(ctx context.Context, nodes []v1.Node, replicaSets []appsv1.ReplicaSet) string {
	// Filter to worker nodes only (same as descheduler)

	workerNodes := []v1.Node{}
	for _, node := range nodes {
		if _, isControlPlane := node.Labels["node-role.kubernetes.io/control-plane"]; !isControlPlane {
			workerNodes = append(workerNodes, node)
		}
	}

	// Create node names list
	nodeNames := make([]string, len(workerNodes))
	for i, node := range workerNodes {
		nodeNames[i] = node.Name
	}
	sort.Strings(nodeNames)

	// Create ReplicaSet specs based on desired replicas (not current pods)
	replicaSetSpecs := make([]string, 0, len(replicaSets))

	for _, rs := range replicaSets {
		// Skip system namespaces to match descheduler behavior
		if isSystemNamespace(rs.Namespace) {
			continue
		}

		// Skip ReplicaSets with 0 desired replicas
		if rs.Spec.Replicas == nil || *rs.Spec.Replicas == 0 {
			continue
		}

		rsKey := fmt.Sprintf("%s/%s", rs.Namespace, rs.Name)
		desiredReplicas := *rs.Spec.Replicas

		// Use only ReplicaSet name and desired replica count for fingerprint
		// This focuses on workload characteristics rather than temporary distribution
		rsSpec := fmt.Sprintf("%s=%d", rsKey, desiredReplicas)
		replicaSetSpecs = append(replicaSetSpecs, rsSpec)
	}

	sort.Strings(replicaSetSpecs)

	// Create cluster specification for hashing
	clusterSpec := fmt.Sprintf("nodes:%s|replicasets:%s",
		strings.Join(nodeNames, ","),
		strings.Join(replicaSetSpecs, ";"))

	s.logger.Info("the cluster spec", "spec", clusterSpec)
	// Return hash for compact storage
	hash := sha256.Sum256([]byte(clusterSpec))
	return fmt.Sprintf("%x", hash)[:16]
}

// isPodEligible checks if a pod should be considered (same logic as descheduler)
func (s *MultiObjectiveScheduler) isPodEligible(pod *v1.Pod) bool {
	// Exclude kube-system namespace pods
	if pod.Namespace == "kube-system" {
		return false
	}

	// Only consider running pods with ReplicaSet owners
	if pod.Status.Phase != v1.PodRunning {
		return false
	}

	// Check if pod has ReplicaSet owner
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			return true
		}
	}

	return false
}

// getReplicaSetKey gets the ReplicaSet key for a pod
func (s *MultiObjectiveScheduler) getReplicaSetKey(pod *v1.Pod) string {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			return fmt.Sprintf("%s/%s", pod.Namespace, owner.Name)
		}
	}
	return fmt.Sprintf("%s/unknown", pod.Namespace)
}

// getAvailableSlotsForReplicaSet gets available slots for a ReplicaSet on a specific node
func (s *MultiObjectiveScheduler) getAvailableSlotsForReplicaSet(solution *deschedulerv1alpha1.OptimizationSolution, rsKey, nodeName string) int {
	for _, rsMovement := range solution.ReplicaSetMovements {
		solutionRSKey := fmt.Sprintf("%s/%s", rsMovement.Namespace, rsMovement.ReplicaSetName)
		if solutionRSKey == rsKey {
			if slots, exists := rsMovement.AvailableSlots[nodeName]; exists {
				return slots
			}
		}
	}
	return 0
}

// tryConsumeSlot attempts to opportunistically consume a scheduling slot with retry
func (s *MultiObjectiveScheduler) tryConsumeSlot(ctx context.Context, hint *deschedulerv1alpha1.SchedulingHint, rsKey, nodeName string) bool {
	config, err := s.getRESTConfig()
	if err != nil {
		s.logger.V(3).Info("Cannot get REST config for slot consumption", "error", err.Error())
		return false
	}

	clientset, err := versioned.NewForConfig(config)
	if err != nil {
		s.logger.V(3).Info("Cannot create clientset for slot consumption", "error", err.Error())
		return false
	}

	// Retry up to 3 times with fresh fetches
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Get fresh hint to avoid conflicts
		freshHint, err := clientset.DeschedulerV1alpha1().SchedulingHints().Get(ctx, hint.Name, metav1.GetOptions{})
		if err != nil {
			s.logger.V(3).Info("Cannot fetch fresh hint for slot consumption",
				"attempt", attempt, "error", err.Error())
			if attempt == maxRetries {
				return false
			}
			continue
		}

		// Find and update the ReplicaSet movement in the top solution only
		if len(freshHint.Spec.Solutions) == 0 {
			s.logger.V(3).Info("No solutions in fresh hint", "attempt", attempt)
			return false
		}

		topSolution := &freshHint.Spec.Solutions[0]
		for i := range topSolution.ReplicaSetMovements {
			rsMovement := &topSolution.ReplicaSetMovements[i]
			solutionRSKey := fmt.Sprintf("%s/%s", rsMovement.Namespace, rsMovement.ReplicaSetName)

			if solutionRSKey == rsKey {
				// Check if slot is still available
				if rsMovement.AvailableSlots[nodeName] > 0 {
					// Consume the slot
					rsMovement.AvailableSlots[nodeName]--
					if rsMovement.ScheduledCount == nil {
						rsMovement.ScheduledCount = make(map[string]int)
					}
					rsMovement.ScheduledCount[nodeName]++

					// Update the hint
					_, err = clientset.DeschedulerV1alpha1().SchedulingHints().Update(ctx, freshHint, metav1.UpdateOptions{})
					if err != nil {
						s.logger.V(3).Info("Failed to update hint after slot consumption",
							"attempt", attempt, "error", err.Error())
						if attempt == maxRetries {
							return false
						}
						continue // Retry with fresh fetch
					}

					s.logger.V(1).Info("Successfully consumed scheduling slot",
						"replicaSet", rsKey,
						"node", nodeName,
						"remainingSlots", rsMovement.AvailableSlots[nodeName],
						"scheduledCount", rsMovement.ScheduledCount[nodeName],
						"hint", hint.Name,
						"attempt", attempt)

					return true
				} else {
					// No slots available
					s.logger.V(3).Info("No slots available on fresh check",
						"attempt", attempt,
						"replicaSet", rsKey,
						"node", nodeName,
						"availableSlots", rsMovement.AvailableSlots[nodeName])
					return false
				}
			}
		}

		// ReplicaSet not found in solution
		s.logger.V(3).Info("ReplicaSet not found in solution",
			"attempt", attempt, "replicaSet", rsKey)
		return false
	}

	return false
}

// generateHintName generates hint name from fingerprint (same as descheduler)
func (s *MultiObjectiveScheduler) generateHintName(fingerprint string) string {
	return fmt.Sprintf("multiobjective-hints-%s", fingerprint)
}

// getRESTConfig gets the REST config for creating custom resource clients
func (s *MultiObjectiveScheduler) getRESTConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fallback to kubeconfig
	s.logger.V(2).Info("In-cluster config not available, trying kubeconfig", "error", err.Error())

	// Try default kubeconfig locations
	kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	return config, nil
}
