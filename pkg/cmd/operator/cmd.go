package operator

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"github.com/openshift/library-go/pkg/controller/fileobserver"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/operator"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

func restartOnConfigChange(ctx context.Context, path string, startingContent []byte) {
	observer, err := fileobserver.NewObserver(1 * time.Second)
	if err != nil {
		panic(err)
	}
	if len(startingContent) == 0 {
		klog.Warningf("No configuration file available")
	}
	observer.AddReactor(func(file string, action fileobserver.ActionType) error {
		os.Exit(0)
		return nil
	}, map[string][]byte{
		path: startingContent,
	}, path)
	observer.Run(ctx.Done())
}

func NewOperator(ctx context.Context) *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the operator",
		Run: func(cmd *cobra.Command, args []string) {
			configBytes, _ := ioutil.ReadFile(configPath)
			go restartOnConfigChange(ctx, configPath, configBytes)
			c := &config.OperatorConfig{}
			if err := yaml.Unmarshal(configBytes, c); err != nil {
				klog.Fatalf("Unable to parse config: %v", err)
			}
			if err := operator.Run(ctx, *c); err != nil {
				klog.Fatal(err)
			}
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to operator config")
	return cmd
}
