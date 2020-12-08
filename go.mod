module github.com/mfojtik/bugzilla-operator

go 1.14

require (
	github.com/boltdb/bolt v1.3.1
	github.com/davecgh/go-spew v1.1.1
	github.com/eparis/bugzilla v0.0.0-20201207155830-bdebb1b9b262
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/gorilla/handlers v1.4.2
	github.com/openshift/build-machinery-go v0.0.0-20200512074546-3744767c4131
	github.com/openshift/library-go v0.0.0-20200615120640-a506fa41d3fb
	github.com/prometheus/common v0.10.0 // indirect
	github.com/prometheus/procfs v0.1.3 // indirect
	github.com/shomali11/commander v0.0.0-20191122162317-51bc574c29ba
	github.com/shomali11/proper v0.0.0-20190608032528-6e70a05688e7
	github.com/sirupsen/logrus v1.6.0
	github.com/slack-go/slack v0.6.4
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	golang.org/x/sys v0.0.0-20200610111108-226ff32320da // indirect
	google.golang.org/protobuf v1.24.0 // indirect
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v0.18.3
	k8s.io/component-base v0.18.3
	k8s.io/klog v1.0.0
)

replace github.com/eparis/bugzilla => github.com/sttts/bugzilla v0.0.0-20201207155830-bdebb1b9b262
