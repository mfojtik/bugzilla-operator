package unfurl

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v33/github"
	operatorslack "github.com/mfojtik/bugzilla-operator/pkg/slack"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/klog"
)

func UnfurlGithubLinks(bus operatorslack.EventBus, client *slack.Client, ghClient *github.Client) error {
	return bus.SubscribeLinkShared(func(ev *slackevents.LinkSharedEvent) {
		unfurls := map[string]slack.Attachment{}

		if ev.User == "U013V0M0H4L" {
			return
		}

		for _, l := range ev.Links {
			// example: https://github.com/kubernetes/kubernetes/pull/96403

			u, err := url.Parse(l.URL)
			if err != nil {
				klog.Infof("failed to parse BZ url %q: %v", l, u)
			}

			if u.Host != "github.com" {
				continue
			}

			comps := strings.Split(strings.TrimLeft(u.Path, "/"), "/")
			if len(comps) != 4 || comps[2] != "pull" {
				continue
			}
			org, repo := comps[0], comps[1]

			id, err := strconv.Atoi(comps[3])
			if err != nil {
				continue
			}
			if id == 0 {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			pr, resp, err := ghClient.PullRequests.Get(ctx, org, repo, id)
			if err != nil {
				klog.Errorf("failed to get %s: %v", u.String(), err)
				continue
			}
			if resp.StatusCode != http.StatusOK || pr == nil || pr.Title == nil {
				continue
			}

			text := fmt.Sprintf(":github: <https://github.com/%s/%s/pull/%d|%s/%s#%d> [*%s*] %s – by %s", org, repo, id, org, repo, id, prState(pr), *pr.Title, prAuthor(pr))
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

func prState(pr *github.PullRequest) string {
	if pr == nil {
		return ""
	}
	if pr.Merged != nil && *pr.Merged == true {
		return "merged"
	}
	if pr.State != nil {
		return *pr.State
	}
	return "unknown"
}

func prAuthor(pr *github.PullRequest) string {
	if pr == nil {
		return ""
	}
	if pr.User == nil {
		return "unknown"
	}
	if pr.User.Login == nil {
		return "unknown"
	}
	return *pr.User.Login
}
