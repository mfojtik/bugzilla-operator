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

		if ev.User == "U013V0M0H4L" {
			return
		}

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
				klog.Infof("bug %d is %v old in cache, refreshing", id, age)
				// refresh
				b, err = bzClient.GetBug(id)
				if err != nil {
					klog.Warningf("failed to refresh bug %d: %v", id, err)
					continue
				}
			}

			text := fmt.Sprintf(":bugzilla: %s [*%s*] %s – %s/%s in *%s* for *%s*/*%s*", bugutil.GetBugURL(*b), b.Status, b.Summary, bugutil.FormatPriority(b.Severity), bugutil.FormatPriority(b.Priority), bugutil.FormatComponent(b.Component), bugutil.FormatVersion(b.Version), bugutil.FormatVersion(b.TargetRelease))
			klog.Infof("Sending unfurl text: %s", text)
			unfurls[l.URL] = slack.Attachment{
				Blocks: slack.Blocks{[]slack.Block{
					slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", text, false, false), nil, nil),
				}},
			}
		}

		if len(unfurls) == 0 {
			return
		}

		_, _, response, err := client.UnfurlMessage(ev.Channel, ev.MessageTimeStamp.String(), unfurls)
		if err != nil {
			klog.Infof("failed unfurling: %v: %s", err, response)
		}
	})
}
