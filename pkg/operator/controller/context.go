package controller

import (
	"context"
	"fmt"
	"time"

	slackgo "github.com/slack-go/slack"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
	"github.com/mfojtik/bugzilla-operator/pkg/slacker"
)

type ControllerContext struct {
	newBugzillaClient             func(debug bool) cache.BugzillaClient
	slackClient, slackDebugClient slack.ChannelClient
	slackerInstance               *slacker.Slacker
	cmClient                      corev1client.ConfigMapInterface
}

func NewControllerContext(newBugzillaClient func(debug bool) cache.BugzillaClient, slackClient, slackDebugClient slack.ChannelClient, slackerInstance *slacker.Slacker, cmClient corev1client.ConfigMapInterface) ControllerContext {
	return ControllerContext{
		newBugzillaClient, slackClient, slackDebugClient, slackerInstance, cmClient,
	}
}

func (c *ControllerContext) NewBugzillaClient(ctx context.Context) cache.BugzillaClient {
	debug, ok := ctx.Value("debug").(bool)
	if ok && debug {
		return c.newBugzillaClient(true)
	}
	return c.newBugzillaClient(false)
}

func (c *ControllerContext) SlackClient(ctx context.Context) slack.ChannelClient {
	debug, ok := ctx.Value("debug").(bool)
	if ok && debug {
		return c.slackDebugClient
	}
	return c.slackClient
}

func (c *ControllerContext) SubscribeBlockAction(blockId string, f func(ctx context.Context, message *slackgo.Container, user *slackgo.User, bzEmail string, action *slackgo.BlockAction)) error {
	if c.slackerInstance == nil {
		return nil
	}

	return c.slackerInstance.SubscribeBlockAction(blockId, func(message *slackgo.Container, user *slackgo.User, action *slackgo.BlockAction) {
		ctx, _ := context.WithTimeout(context.Background(), time.Second*10)
		f(ctx, message, user, slack.SlackEmailToBugzilla(user.Profile.Email), action)
	})
}

func (c *ControllerContext) GetPersistentValue(ctx context.Context, key string) (string, error) {
	cm, err := c.cmClient.Get(ctx, "state", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	return cm.Data[key], nil
}

func (c *ControllerContext) SetPersistentValue(ctx context.Context, key, value string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		debug, ok := ctx.Value("debug").(bool)
		if ok && debug {
			c.slackClient.MessageAdminChannel(fmt.Sprintf("Faking SetPersistentValue(%q, %q)", key, value))
			return nil
		}

		klog.Infof("Setting %s=%q", key, value)

		cm, err := c.cmClient.Get(ctx, "state", metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				cm = &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name: "state",
					},
					Data: map[string]string{key: value},
				}
				if _, err := c.cmClient.Create(ctx, cm, metav1.CreateOptions{}); err != nil {
					return err
				}
			} else {
				return err
			}
		}

		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data[key] = value

		_, err = c.cmClient.Update(ctx, cm, metav1.UpdateOptions{})
		return err
	})
}
