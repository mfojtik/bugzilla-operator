package operator

import (
	"context"

	"github.com/davecgh/go-spew/spew"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/stalecontroller"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

func anonymizeConfig(in *config.OperatorConfig) config.OperatorConfig {
	out := *in
	out.Credentials = config.Credentials{}
	return out
}

func Run(ctx context.Context, operatorConfig config.OperatorConfig) error {
	klog.Infof("Starting Operator\nConfig: %s\n", spew.Sdump(anonymizeConfig(&operatorConfig)))

	slackClient := slack.NewClient(operatorConfig.SlackChannel, operatorConfig.Credentials.DecodedSlackToken())
	recorder := slack.NewRecorder(slackClient, "BugzillaOperator", operatorConfig.SlackChannel)

	staleController := stalecontroller.NewStaleController(operatorConfig, slackClient, recorder)
	go staleController.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
