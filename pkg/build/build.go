package build

import (
	"context"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vdemeester/buildkit-tekton/pkg/config"
	"github.com/vdemeester/buildkit-tekton/pkg/tekton"
)

const (
	localNameDockerfile = "dockerfile" // This is there to make it work with docker build -f …
	keyFilename         = "filename"
	defaultTaskName     = "task.yaml"
)

// Build is the "core" of the frontend.
// From the client, it parses the options, get the resources, translate them to BuildKit LLB
// and "solve" it (aka send it to BuildKit).
func Build(ctx context.Context, c client.Client) (*client.Result, error) {
	logrus.Infof("Hellloooooo")
	// Handle opts AND build-args
	cfg, err := config.Parse(c.BuildOpts())
	if err != nil {
		return nil, errors.Wrap(err, "failed loading options")
	}

	ctx = cfg.ToContext(ctx)
	resource, err := GetTektonResource(ctx, c)
	if err != nil {
		return nil, errors.Wrap(err, "getting tekton resource")
	}
	contextResources, err := GetContextResources(ctx, c)
	if err != nil {
		return nil, errors.Wrap(err, "getting context resource")
	}
	st, err := tekton.TektonToLLB(c)(ctx, resource, contextResources)
	if err != nil {
		return nil, err
	}

	def, err := st.Marshal(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal local source")
	}
	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve dockerfile")
	}
	ref, err := res.SingleRef()
	if err != nil {
		return nil, err
	}

	res.SetRef(ref)

	return res, nil
}

// GetTektonResource reads the specified tekton resource (TaskRun, Pipeline today), and
// returns it as a string.
func GetTektonResource(ctx context.Context, c client.Client) (string, error) {
	opts := c.BuildOpts().Opts
	filename := opts[keyFilename]
	if filename == "" {
		filename = defaultTaskName
	}

	name := "load tekton"
	if filename != "task.yaml" {
		name += " from " + filename
	}

	src := llb.Local(localNameDockerfile,
		// llb.IncludePatterns([]string{filename, "*"}),
		llb.SessionID(c.BuildOpts().SessionID),
		// llb.SharedKeyHint(defaultTaskName),
		dockerfile2llb.WithInternalName(name),
	)

	def, err := src.Marshal(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal local source")
	}

	var dtDockerfile []byte
	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to resolve tekton.yaml")
	}

	ref, err := res.SingleRef()
	if err != nil {
		return "", err
	}

	dtDockerfile, err = ref.ReadFile(ctx, client.ReadRequest{
		Filename: filename,
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to read tekton yaml")
	}

	return string(dtDockerfile), nil
}

func GetContextResources(ctx context.Context, c client.Client) ([]string, error) {
	resources := []string{}
	buildContext := llb.Local("context",
		llb.IncludePatterns([]string{"*.yml", "*.yaml"}),
		llb.SessionID(c.BuildOpts().SessionID),
		dockerfile2llb.WithInternalName("context"),
	)
	def, err := buildContext.Marshal(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load context files")
	}
	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to load context files")
	}

	ref, err := res.SingleRef()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load context files")
	}

	dirs, err := ref.ReadDir(ctx, client.ReadDirRequest{Path: ""})
	if err != nil {
		return nil, errors.Wrap(err, "failed to load context files")
	}
	logrus.Infof("len(dirs): %d", len(dirs))
	for _, d := range dirs {
		logrus.Infof("dir: %+v", d)
		data, err := ref.ReadFile(ctx, client.ReadRequest{
			Filename: d.Path,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read %s from context files", d.Path)
		}
		resources = append(resources, string(data))
	}
	return resources, nil
}
