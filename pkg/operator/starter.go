package operator

import (
	"context"

	"github.com/davecgh/go-spew/spew"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/stalecontroller"

	"github.com/openshift/library-go/pkg/operator/events"
)

func anonymizeConfig(in *config.OperatorConfig) config.OperatorConfig {
	out := *in
	out.Credentials = config.BugzillaCredentials{}
	return out
}

func Run(ctx context.Context, operatorConfig config.OperatorConfig) error {
	klog.Infof("Starting Operator\nConfig: %s\n", spew.Sdump(anonymizeConfig(&operatorConfig)))

	recorder := events.NewLoggingEventRecorder("BugzillaStaleBugs")

	staleController := stalecontroller.NewStaleController(operatorConfig, recorder)
	go staleController.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
