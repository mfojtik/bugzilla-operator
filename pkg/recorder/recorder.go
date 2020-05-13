package recorder

import (
	"github.com/openshift/library-go/pkg/operator/events"
)

type Recorder struct{}

func (r *Recorder) Event(reason, message string) {
	panic("implement me")
}

func (r *Recorder) Eventf(reason, messageFmt string, args ...interface{}) {
	panic("implement me")
}

func (r *Recorder) Warning(reason, message string) {
	panic("implement me")
}

func (r *Recorder) Warningf(reason, messageFmt string, args ...interface{}) {
	klog.
		panic("implement me")
}

func (r *Recorder) ForComponent(componentName string) events.Recorder {
	return r
}

func (r *Recorder) WithComponentSuffix(componentNameSuffix string) events.Recorder {
	return r
}

func (r *Recorder) ComponentName() string {
	return "Operator"
}

func (r *Recorder) Shutdown() {}
