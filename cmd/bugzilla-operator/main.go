package main

import (
	"context"
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cmd/operator"
	"github.com/mfojtik/bugzilla-operator/pkg/version"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()

	logrus.SetOutput(logs.KlogWriter{})
	switch {
	case klog.V(9) == true:
		logrus.SetLevel(logrus.TraceLevel)
	case klog.V(8) == true:
		logrus.SetLevel(logrus.DebugLevel)
	case klog.V(4) == true:
		logrus.SetLevel(logrus.InfoLevel)
	case klog.V(2) == true:
		logrus.SetLevel(logrus.WarnLevel)
	default:
		logrus.SetLevel(logrus.ErrorLevel)
	}

	ctx := context.TODO()

	command := NewOperatorCommand(ctx)
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func NewOperatorCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bugzilla-operator",
		Short: "An operator that operates bugzilla numbers and automatically improve product quality",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
			os.Exit(1)
		},
	}

	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	cmd.AddCommand(operator.NewOperator(ctx))

	return cmd
}
