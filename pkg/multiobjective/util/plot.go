package util

import (
	"fmt"

	"os"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
	"sigs.k8s.io/scheduler-plugins/pkg/multiobjective/framework"
)

// PlotResults creates a scatter plot comparing the true Pareto front of the given Problem
// with the final population resulted from the algorithm.
func PlotResults(problem framework.Problem, finalPop []framework.Individual, algorithmName string) error {
	// Create scatter chart
	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: fmt.Sprintf("%s Results for %s Benchmark", algorithmName, problem.Name()),
		}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true)}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
		charts.WithInitializationOpts(opts.Initialization{
			Theme: types.ThemeWesteros,
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Name: "f1(x)",
			SplitLine: &opts.SplitLine{
				Show: opts.Bool(true),
			},
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Name: "f2(x)",
			SplitLine: &opts.SplitLine{
				Show: opts.Bool(true),
			},
		}),
	)

	trueParetoFront := problem.TrueParetoFront(100)
	trueX := make([]opts.ScatterData, len(trueParetoFront))
	for i, p := range trueParetoFront {
		trueX[i] = opts.ScatterData{
			Value:      p,
			Symbol:     "circle",
			SymbolSize: 10,
		}
	}

	foundX := make([]opts.ScatterData, len(finalPop))
	for i, ind := range finalPop {
		foundX[i] = opts.ScatterData{
			Value:      []float64{ind.Objectives[0], ind.Objectives[1]},
			Symbol:     "triangle",
			SymbolSize: 10,
		}
	}

	// Add data series
	scatter.AddSeries("True Pareto Front", trueX).
		AddSeries(fmt.Sprintf("%s Solutions", algorithmName), foundX).
		SetSeriesOptions(
			charts.WithLabelOpts(opts.Label{
				Show: opts.Bool(false),
			}),
			charts.WithEmphasisOpts(opts.Emphasis{}),
		)

	// Create HTML file
	f, err := os.Create(fmt.Sprintf("%s_%s_results.html", problem.Name(), algorithmName))
	if err != nil {
		return err
	}
	defer f.Close()

	return scatter.Render(f)
}
