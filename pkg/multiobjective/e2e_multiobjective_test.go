package multiobjective

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	schedulerName = "multi-objective-scheduler"
)

type contextKey string

func TestSchedulerObjectives(t *testing.T) {
	testenv = env.New()
	path := conf.ResolveKubeConfigFile()
	cfg := envconf.NewWithKubeConfig(path)
	testenv = env.NewWithConfig(cfg)

	namespace := "default"

	// Test 1: Power-Focused Scenario
	powerFeature := features.New("power objective behavior").
		WithLabel("type", "multi-objective").
		WithSetup("create nodes and load-generating pods", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client := cfg.Client()
			envfuncs.CreateNamespace(namespace)

			// Create nodes with different profiles
			nodes := []*corev1.Node{
				createTestNode("low-util-node", 140.0, 200.0, 8, 32),    // 8 CPU cores, 32GB RAM
				createTestNode("med-util-node", 140.0, 200.0, 16, 64),   // 16 CPU cores, 64GB RAM
				createTestNode("high-util-node", 140.0, 200.0, 32, 128), // 32 CPU cores, 128GB RAM
			}

			for _, node := range nodes {
				assert.NoError(t, client.Resources().Create(ctx, node))
			}

			waitForNodesReady(client, nodes, 10*time.Second)

			// Then create load-generating pods
			loadPods := []*corev1.Pod{
				createLoadPod("load-low", namespace, "low-util-node", "800m"),    // 10% of 8 cores
				createLoadPod("load-med", namespace, "med-util-node", "3200m"),   // 40% of 8 cores
				createLoadPod("load-high", namespace, "high-util-node", "5600m"), // 70% of 8 cores
			}

			for _, pod := range loadPods {
				assert.NoError(t, client.Resources().Create(ctx, pod))
			}

			// Wait for pods to be "running" (KWOK will simulate this)
			time.Sleep(5 * time.Second)

			// Store nodes in context for cleanup
			ctxWithVal := context.WithValue(ctx, "load-pods", loadPods)
			return context.WithValue(ctxWithVal, "test-nodes", nodes)
		}).
		Assess("verify power-aware placement", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client := cfg.Client()

			// Create a small pod - should avoid low-util node due to power penalty
			pod := createTestPod("test-pod-small", namespace, "100m", "100Mi")
			assert.NoError(t, client.Resources().Create(ctx, pod))

			// Wait for scheduling
			time.Sleep(5 * time.Second)

			// Verify pod landed on med-util-node (best power efficiency)
			pod = &corev1.Pod{}
			assert.NoError(t, client.Resources().Get(ctx, "test-pod-small", "default", pod))
			assert.Equal(t, "med-util-node", pod.Spec.NodeName)

			// Store pod in context for cleanup
			return context.WithValue(ctx, "test-pod", pod)
		}).
		Teardown(deleteResources).
		Feature()

	// Run all tests
	testenv.Test(t, powerFeature)
}

func createTestNode(name string, pIdle, pBusy float64, cpuCores int64, memoryGB int64) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"node.alpha.kubernetes.io/ttl": "0",
				"kwok.x-k8s.io/node":           "fake",

				"multiobjective.x-k8s.io/power-idle": fmt.Sprintf("%.2f", pIdle),
				"multiobjective.x-k8s.io/power-busy": fmt.Sprintf("%.2f", pBusy),
			},
			Labels: map[string]string{
				"beta.kubernetes.io/arch":       "amd64",
				"beta.kubernetes.io/os":         "linux",
				"kubernetes.io/arch":            "amd64",
				"kubernetes.io/hostname":        name,
				"kubernetes.io/os":              "linux",
				"kubernetes.io/role":            "agent",
				"node-role.kubernetes.io/agent": "",
				"type":                          "kwok",
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{
					Key:    "kwok.x-k8s.io/node",
					Value:  "fake",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(cpuCores, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(memoryGB*1024*1024*1024, resource.BinarySI),
				corev1.ResourcePods:   *resource.NewQuantity(110, resource.DecimalSI),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(cpuCores, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(memoryGB*1024*1024*1024, resource.BinarySI),
				corev1.ResourcePods:   *resource.NewQuantity(110, resource.DecimalSI),
			},
			NodeInfo: corev1.NodeSystemInfo{
				Architecture:            "amd64",
				BootID:                  "",
				ContainerRuntimeVersion: "",
				KernelVersion:           "",
				KubeProxyVersion:        "fake",
				KubeletVersion:          "fake",
				MachineID:               "",
				OperatingSystem:         "linux",
				OSImage:                 "",
				SystemUUID:              "",
			},
			Phase: corev1.NodeRunning,
		},
	}
}

func createLoadPod(name, namespace, nodeName, cpu string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName, // Direct node assignment
			Containers: []corev1.Container{
				{
					Name:  "load-generator",
					Image: "nginx", // KWOK will simulate this
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpu),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpu),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
}

func createTestPod(name, namespace, cpu, memory string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			SchedulerName: schedulerName,
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpu),
							corev1.ResourceMemory: resource.MustParse(memory),
						},
					},
				},
			},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"kwok"},
									},
								},
							},
						},
					},
				},
			},
			Tolerations: []corev1.Toleration{
				{
					Key:      "kwok.x-k8s.io/node",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
	}
}

func deleteResources(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
	client := c.Client()

	if pod, ok := ctx.Value("test-pod").(*corev1.Pod); ok {
		_ = client.Resources().Delete(ctx, pod)
	}

	if pods, ok := ctx.Value("load-pods").([]*corev1.Pod); ok {
		for _, pod := range pods {
			_ = client.Resources().Delete(ctx, pod)
		}
	}

	if nodes, ok := ctx.Value("test-nodes").([]*corev1.Node); ok {
		for _, node := range nodes {
			_ = client.Resources().Delete(ctx, node)
		}
	}

	return ctx
}

func waitForNodesReady(client klient.Client, nodes []*corev1.Node, timeout time.Duration) error {
	for _, node := range nodes {
		name := node.Name
		err := wait.For(func(ctx context.Context) (done bool, err error) {
			node := &corev1.Node{}
			err = client.Resources().Get(ctx, name, "", node)
			if err != nil {
				return false, err
			}

			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
			return false, nil
		}, wait.WithTimeout(timeout))

		if err != nil {
			return fmt.Errorf("timeout waiting for node %s to be ready: %v", name, err)
		}
	}
	return nil
}
