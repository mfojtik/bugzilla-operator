module github.com/mfojtik/bugzilla-operator

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/eparis/bugzilla v0.0.0-20200513185855-1f6d55c0d229
	github.com/openshift/build-machinery-go v0.0.0-20200512074546-3744767c4131
	github.com/openshift/library-go v0.0.0-20200512120242-21a1ff978534
	github.com/robfig/cron/v3 v3.0.0
	github.com/slack-go/slack v0.6.4
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	k8s.io/component-base v0.18.2
	k8s.io/klog v1.0.0
)

replace github.com/eparis/bugzilla => github.com/mfojtik/bugzilla v0.0.0-20200513191148-d8847633ba44
