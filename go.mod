module github.com/mfojtik/bugzilla-operator

go 1.14

require (
	github.com/boltdb/bolt v1.3.1
	github.com/davecgh/go-spew v1.1.1
	github.com/eparis/bugzilla v0.0.0-20200518124032-4b8cbef6916b
	github.com/gorilla/handlers v1.4.2
	github.com/openshift/build-machinery-go v0.0.0-20200512074546-3744767c4131
	github.com/openshift/library-go v0.0.0-20200512120242-21a1ff978534
	github.com/robfig/cron/v3 v3.0.0
	github.com/shomali11/commander v0.0.0-20191122162317-51bc574c29ba
	github.com/shomali11/proper v0.0.0-20190608032528-6e70a05688e7
	github.com/slack-go/slack v0.6.4
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	k8s.io/component-base v0.18.2
	k8s.io/klog v1.0.0
)

replace github.com/eparis/bugzilla => github.com/mfojtik/bugzilla v0.0.0-20200611073413-8c3af3656d03
