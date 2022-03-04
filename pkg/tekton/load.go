package tekton

import (
	// "fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	k8scheme "k8s.io/client-go/kubernetes/scheme"
)

type TaskRun struct {
	main  *v1beta1.TaskRun
	tasks map[string]*v1beta1.Task
}

type PipelineRun struct {
	main      *v1beta1.PipelineRun
	tasks     map[string]*v1beta1.Task
	pipelines map[string]*v1beta1.Pipeline
}

func readResources(main string, additionals []string) (interface{}, error) {
	s := k8scheme.Scheme
	if err := v1beta1.AddToScheme(s); err != nil {
		return nil, err
	}
	if l := len(strings.Split(strings.Trim(main, "-"), "---")); l > 1 {
		return nil, errors.New("Multiple resource in the main resource not supported")
	}
	obj, err := parseTektonYAML(main)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal %v", main)
	}
	switch o := obj.(type) {
	case *v1beta1.TaskRun:
		return populateTaskRun(o, additionals)
	case *v1beta1.PipelineRun:
		return populatePipelineRun(o, additionals)
	default:
		return nil, errors.Errorf("Document doesn't look like a tekton resource we can Resolve. %s", main)
	}
}

func populateTaskRun(tr *v1beta1.TaskRun, additionals []string) (TaskRun, error) {
	r := TaskRun{
		main: tr,
	}
	for _, data := range additionals {
		for _, doc := range strings.Split(strings.Trim(data, "-"), "---") {
			obj, err := parseTektonYAML(doc)
			if err != nil {
				return r, errors.Wrapf(err, "failed to unmarshal %v", doc)
			}
			switch o := obj.(type) {
			case *v1beta1.Task:
				r.tasks[o.Name] = o
			default:
				logrus.Infof("Skipping document not looking like a tekton resource we can Resolve.")
			}
		}
	}
	return r, nil
}

func populatePipelineRun(pr *v1beta1.PipelineRun, additionals []string) (PipelineRun, error) {
	r := PipelineRun{
		main: pr,
	}
	for _, data := range additionals {
		for _, doc := range strings.Split(strings.Trim(data, "-"), "---") {
			obj, err := parseTektonYAML(doc)
			if err != nil {
				return r, errors.Wrapf(err, "failed to unmarshal %v", doc)
			}
			switch o := obj.(type) {
			case *v1beta1.Task:
				r.tasks[o.Name] = o
			case *v1beta1.Pipeline:
				r.pipelines[o.Name] = o
			default:
				logrus.Infof("Skipping document not looking like a tekton resource we can Resolve.")
			}
		}
	}
	return r, nil
}

func parseTektonYAML(s string) (interface{}, error) {
	decoder := k8scheme.Codecs.UniversalDeserializer()
	obj, _, err := decoder.Decode([]byte(s), nil, nil)
	if err != nil {
		return nil, err
	}
	return obj, nil
}
