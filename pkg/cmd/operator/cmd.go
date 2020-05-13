package operator

import (
	"context"
	"io/ioutil"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/operator"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

func NewOperator(ctx context.Context) *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the operator",
		Run: func(cmd *cobra.Command, args []string) {
			configBytes, err := ioutil.ReadFile(configPath)
			if err != nil {
				klog.Fatalf("Unable to read config %q: %v", configPath, err)
			}
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
