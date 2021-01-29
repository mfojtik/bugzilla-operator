package unfurl

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	operatorslack "github.com/mfojtik/bugzilla-operator/pkg/slack"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/klog"
)

func UnfurlBugzillaLinks(bus operatorslack.EventBus, client *slack.Client, bzClient cache.BugzillaClient) error {
	return bus.SubscribeLinkShared(func(ev *slackevents.LinkSharedEvent) {
		unfurls := map[string]slack.Attachment{}

		for _, l := range ev.Links {
			// example: https://bugzilla.redhat.com/show_bug.cgi?id=1873114

			u, err := url.Parse(l.URL)
			if err != nil {
				klog.Infof("failed to parse BZ url %q: %v", l, u)
			}

			if u.Host != "bugzilla.redhat.com" || strings.TrimLeft(u.Path, "/") != "show_bug.cgi" {
				continue
			}

			idString := u.Query().Get("id")
			if len(idString) == 0 {
				continue
			}

			id, err := strconv.Atoi(idString)
			if err != nil {
				continue
			}

			if id == 0 {
				continue
			}

			b, age, err := bzClient.GetCachedBug(id, "")
			if err != nil {
				klog.Warningf("failed to get cached bug %d: %v", id, err)
				continue
			}

			if age > time.Minute*10 {
				// refresh
				b, err = bzClient.GetBug(id)
				if err != nil {
					klog.Warningf("failed to refresh bug %d: %v", id, err)
					continue
				}
			}

			target := "---"
			if len(b.TargetRelease) > 0 {
				target = b.TargetRelease[0]
			}
			text := fmt.Sprintf(":bugzilla: *%d* – %s – %s/%s @ %s / %s", id, b.Summary, bugutil.FormatPriority(b.Severity), bugutil.FormatPriority(b.Priority), b.Component, target)
			unfurls[l.URL] = slack.Attachment{
				Blocks: slack.Blocks{[]slack.Block{
					slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", text, true, false), nil, nil),
				}},
			}
		}

		_, _, _, err := client.UnfurlMessage(ev.Channel, ev.MessageTimeStamp.String(), unfurls)
		if err != nil {
			klog.Infof("failed unfurling: %v", err)
		}
	})
}
