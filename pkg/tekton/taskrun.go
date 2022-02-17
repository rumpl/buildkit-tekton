package tekton

import (
	"context"
	"fmt"

	"github.com/docker/distribution/reference"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

type pstep struct {
	name       string
	image      string // might be ref
	mounts     []mountOptionFn
	runOptions []llb.RunOption
	workspaces map[string]llb.MountOption
}

type mountOptionFn func(llb.State) llb.RunOption

func TaskRunToLLB(ctx context.Context, c client.Client, tr *v1beta1.TaskRun) (llb.State, error) {
	// Validation
	if tr.Name == "" && tr.GenerateName != "" {
		tr.Name = tr.GenerateName + "generated"
	}
	if err := tr.Validate(ctx); err != nil {
		return llb.State{}, errors.Wrapf(err, "validation failed for Taskrun %s", tr.Name)
	}
	if tr.Spec.TaskSpec == nil {
		return llb.State{}, errors.New("TaskRef not supported")
	}

	// Interpolation
	// TODO(vdemeester) implement this

	// Execution
	workspaces := map[string]llb.MountOption{}
	for _, w := range tr.Spec.Workspaces {
		workspaces[w.Name] = llb.AsPersistentCacheDir(tr.Name+"/"+w.Name, llb.CacheMountShared)
	}
	steps, err := taskSpecToPSteps(ctx, c, *tr.Spec.TaskSpec, tr.Name, workspaces)
	if err != nil {
		return llb.State{}, errors.Wrap(err, "couldn't translate TaskSpec to builtkit llb")
	}
	logrus.Infof("steps: %+v", steps)
	stepStates, err := pstepToState(c, steps, []llb.RunOption{})
	if err != nil {
		return llb.State{}, err
	}
	return stepStates[len(stepStates)-1], nil
}

func taskSpecToPSteps(ctx context.Context, c client.Client, t v1beta1.TaskSpec, name string, workspaces map[string]llb.MountOption) ([]pstep, error) {
	steps := make([]pstep, len(t.Steps))
	cacheDirName := name + "/results"
	taskWorkspaces := map[string]llb.MountOption{}
	for _, w := range t.Workspaces {
		taskWorkspaces["/workspace/"+w.Name] = workspaces[w.Name]
	}
	logrus.Infof("+taskWorkspaces: %+v", taskWorkspaces)
	for i, step := range t.Steps {
		ref, err := reference.ParseNormalizedNamed(step.Image)
		if err != nil {
			return steps, err
		}
		runOptions := []llb.RunOption{
			llb.IgnoreCache,
			llb.WithCustomName(name + "/" + step.Name),
		}
		if step.Script != "" {
			return steps, errors.New("script not supported")
		} else {
			runOptions = append(runOptions,
				llb.Args(append(step.Command, step.Args...)),
			)
		}
		if step.WorkingDir != "" {
			runOptions = append(runOptions,
				llb.With(llb.Dir(step.WorkingDir)),
			)
		}
		mounts := []mountOptionFn{
			func(state llb.State) llb.RunOption {
				return llb.AddMount("/tekton/results", state, llb.AsPersistentCacheDir(cacheDirName, llb.CacheMountShared))
			},
		}
		steps[i] = pstep{
			name:       step.Name,
			image:      ref.String(),
			runOptions: runOptions,
			mounts:     mounts,
			workspaces: workspaces,
		}
	}
	return steps, nil
}

func pstepToState(c client.Client, steps []pstep, additionnalMounts []llb.RunOption) ([]llb.State, error) {
	stepStates := make([]llb.State, len(steps))
	for i, step := range steps {
		logrus.Infof("step-%d: %s", i, step.name)
		runOptions := step.runOptions
		mounts := make([]llb.RunOption, len(step.mounts))
		for i, m := range step.mounts {
			mounts[i] = m(stepStates[i])
		}
		// If not the first step, we need to create the chain to execute things in sequence
		if i > 0 {
			// TODO decide what to mount exactly
			targetMount := fmt.Sprintf("/tekton-results/%d", i-1)
			mounts = append(mounts,
				llb.AddMount(targetMount, stepStates[i-1], llb.SourcePath("/"), llb.Readonly),
			)
		}
		for workspacePath, workspaceOptions := range step.workspaces {
			logrus.Infof("Mount in %s: %+v", workspacePath, workspaceOptions)
			mounts = append(mounts,
				llb.AddMount(workspacePath, stepStates[i], workspaceOptions),
			)
		}
		runOptions = append(runOptions, mounts...)
		runOptions = append(runOptions, additionnalMounts...)
		state := llb.
			Image(step.image, llb.WithMetaResolver(c), llb.WithCustomName("load metadata from +"+step.image)).
			Run(runOptions...).
			Root()
		stepStates[i] = state
	}
	return stepStates, nil
}
