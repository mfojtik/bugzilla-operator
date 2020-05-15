package slack

import (
	"fmt"

	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/klog"
)

type Recorder struct {
	client          Client
	component       string
	targetUserEmail string
}

var _ events.Recorder = &Recorder{}

func NewRecorder(client Client, component, user string) events.Recorder {
	return &Recorder{
		client:          client,
		component:       component,
		targetUserEmail: user,
	}
}

func (r *Recorder) Event(reason, message string) {
	msg := fmt.Sprintf("[_%s#%s_] %s", r.component, reason, message)
	if err := r.client.MessageEmail(r.targetUserEmail, msg); err != nil {
		klog.Warningf("Failed to send: %s", msg)
	}
}

func (r *Recorder) Eventf(reason, messageFmt string, args ...interface{}) {
	r.Event(reason, fmt.Sprintf(messageFmt, args...))
}

func (r *Recorder) Warning(reason, message string) {
	msg := fmt.Sprintf(":warning: [_%s#%s_] %s", r.component, reason, message)
	if err := r.client.MessageEmail(r.targetUserEmail, msg); err != nil {
		klog.Warningf("Failed to send: %s", msg)
	}
}

func (r *Recorder) Warningf(reason, messageFmt string, args ...interface{}) {
	r.Warningf(reason, fmt.Sprintf(messageFmt, args...))
}

func (r *Recorder) ForComponent(componentName string) events.Recorder {
	newRecorder := *r
	newRecorder.component = componentName
	return &newRecorder
}

func (r *Recorder) WithComponentSuffix(componentNameSuffix string) events.Recorder {
	return r.ForComponent(r.component + "_" + componentNameSuffix)
}

func (r *Recorder) ComponentName() string {
	return r.component
}

func (r *Recorder) Shutdown() {}
