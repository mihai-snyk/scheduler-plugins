package multiobjective

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/scheduler-plugins/apis/config"
)

type MultiObjective struct {
	handle framework.Handle
}

var _ framework.PreScorePlugin = &MultiObjective{}
var _ framework.ScorePlugin = &MultiObjective{}

const (
	Name = "MultiObjective"
)

func New(ctx context.Context, obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	logger := klog.FromContext(ctx)
	logger.V(5).Info("creating instance of MultiObjective")

	args, ok := obj.(*config.MultiObjectiveArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type MultiObjectiveArgs, got %T", obj)
	}
	logger.V(5).Info(fmt.Sprintf("plugin MultiObjective called with args %v", args.ObjectiveWeights))

	plugin := &MultiObjective{
		handle: handle,
	}

	return plugin, nil
}

func (p *MultiObjective) Name() string {
	return Name
}

func (p *MultiObjective) PreScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodes []*framework.NodeInfo) *framework.Status {
	logger := klog.FromContext(ctx)
	logger.V(5).Info(fmt.Sprintf("running the PreScore for %s plugin!", p.Name()))

	return framework.NewStatus(framework.Success, "")
}

// Just read from the state and return the scores accordingly.
func (p *MultiObjective) Score(ctx context.Context, state *framework.CycleState, po *v1.Pod, nodeName string) (int64, *framework.Status) {
	logger := klog.FromContext(ctx)
	logger.V(5).Info(fmt.Sprintf("running the Score for %s plugin!", p.Name()))

	if nodeName == "kwok-node-1" {
		return framework.MaxNodeScore, framework.NewStatus(framework.Success, "")
	}
	return framework.MinNodeScore, framework.NewStatus(framework.Success, "")
}

func (p *MultiObjective) ScoreExtensions() framework.ScoreExtensions {
	return nil
}
