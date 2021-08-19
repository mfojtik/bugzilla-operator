package unfurl

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/davecgh/go-spew/spew"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/klog"

	operatorslack "github.com/mfojtik/bugzilla-operator/pkg/slack"
)

func UnfurlJiraLinks(bus operatorslack.EventBus, client *slack.Client, jiraClient *jira.Client) error {
	return bus.SubscribeLinkShared(func(ev *slackevents.LinkSharedEvent) {
		unfurls := map[string]slack.Attachment{}

		if ev.User == "U013V0M0H4L" {
			return
		}

		for _, l := range ev.Links {
			// example: https://issues.redhat.com/browse/API-1299

			u, err := url.Parse(l.URL)
			if err != nil {
				klog.Infof("failed to parse BZ url %q: %v", l, u)
			}

			if u.Host != "issues.redhat.com" {
				continue
			}

			comps := strings.Split(strings.TrimLeft(u.Path, "/"), "/")
			if len(comps) != 2 || comps[1] != "browse" {
				continue
			}
			id := comps[1]
			if len(id) == 0 {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			issue, _, err := jiraClient.Issue.GetWithContext(ctx, id, nil)
			if err != nil {
				klog.Errorf("failed to get %s: %v", u.String(), err)
				continue
			}

			text := fmt.Sprintf(":jira-dumpster-fire: <https://issues.redhat.com/browse/%s|#%s> %s – by %s", id, id, issue.Expand, "unknown")
			klog.Infof("Sending unfurl text: %s\n\n%s", text, spew.Sdump(issue))
			unfurls[l.URL] = slack.Attachment{
				Blocks: slack.Blocks{[]slack.Block{
					slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", text, false, false), nil, nil),
				}},
			}
		}

		if len(unfurls) == 0 {
			return
		}

		/*_, _, response, err := client.UnfurlMessage(ev.Channel, ev.MessageTimeStamp.String(), unfurls)
		if err != nil {
			klog.Infof("failed unfurling: %v: %s", err, response)
		}*/
	})
}
