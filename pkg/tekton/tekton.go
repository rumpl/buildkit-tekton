package tekton

import (
	"context"
	"fmt"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

type types struct {
	PipelineRuns []*v1beta1.PipelineRun
	TaskRuns     []*v1beta1.TaskRun
	Tasks        []*v1beta1.Task
	Pipelines    []*v1beta1.Pipeline
}

// TektonToLLB returns a function that converts a string representing a Tekton resource
// into a BuildKit LLB State.
// Only support TaskRun with embedded Task to start.
func TektonToLLB(c client.Client) func(context.Context, string, []string) (llb.State, error) {
	return func(ctx context.Context, l string, refs []string) (llb.State, error) {
		run, err := readResources(l, refs)
		if err != nil {
			return llb.State{}, errors.Wrap(err, "failed to read resources")
		}

		switch r := run.(type) {
		case TaskRun:
			return TaskRunToLLB(ctx, c, r.main)
		case PipelineRun:
			return PipelineRunToLLB(ctx, c, r.main)
		default:
			return llb.State{}, fmt.Errorf("Invalid state")
		}
	}
}
