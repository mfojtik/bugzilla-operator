package ideas

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"

	"github.com/mfojtik/bugzilla-operator/pkg/slacker"
)

type Controller struct {
	ctx controller.ControllerContext
}

type Idea struct {
	Team          string    `json:"team"`
	Title         string    `json:"title"`
	Clarification string    `json:"clarification"`
	From          string    `json:"from"`
	Timestamp     time.Time `json:"timestamp"`
}

type IdeaList struct {
	Items []Idea `json:"items"`
}

func replyInThread(r *slacker.ReplyDefaults) {
	r.ThreadResponse = true
}

func New(ctx controller.ControllerContext) *Controller {
	return &Controller{ctx: ctx}
}

// newIdea parse the description given in slack message and check the format is right.
// then it returns idea title and clarification.
func newIdeaFromDescription(s string) (*Idea, error) {
	parts := strings.SplitN(s, "because", 1)
	if len(parts) != 2 {
		return nil, fmt.Errorf("the description must contain 'because' word. eg. 'Improve thing X *because* we need more stability' (%q)", s)
	}
	return &Idea{
		Title:         strings.TrimSpace(parts[0]),
		Clarification: strings.TrimSpace(parts[1]),
		Timestamp:     time.Now(),
	}, nil
}

func (c *Controller) getList(ctx context.Context) *IdeaList {
	curr, _ := c.ctx.GetPersistentValue(ctx, "ideas")
	if len(curr) == 0 {
		return nil
	}
	var result IdeaList
	if err := json.Unmarshal([]byte(curr), &result); err != nil {
		return nil
	}
	return &result
}

func (c *Controller) updateList(ctx context.Context, new IdeaList) error {
	b, _ := json.Marshal(new)
	return c.ctx.SetPersistentValue(ctx, "ideas", string(b))
}

func (c *Controller) addToList(ctx context.Context, idea *Idea) error {
	curr := c.getList(ctx)
	if curr == nil {
		curr = &IdeaList{Items: []Idea{*idea}}
	}
	newList := *curr
	newList.Items = append(newList.Items, *idea)
	return c.updateList(ctx, newList)
}

func (c *Controller) resetListForTeam(ctx context.Context, team string) error {
	curr := c.getList(ctx)
	if curr == nil {
		return fmt.Errorf("there are no ideas stored for team %q", team)
	}
	newList := &IdeaList{Items: []Idea{}}
	for i := range curr.Items {
		if curr.Items[i].Team == team {
			continue
		}
		newList.Items = append(newList.Items, curr.Items[i])
	}
	return c.updateList(ctx, *newList)
}

func (c *Controller) AddCommands(s *slacker.Slacker) {
	s.Command("idea-for-team <team> <description>", &slacker.CommandDefinition{
		Description: "Record an idea for given team. Ideas will be triaged during planning meeting. Description must contain the 'because' word.",
		Handler: func(req slacker.Request, w slacker.ResponseWriter) {
			teamName := req.StringParam("team", "")
			if len(teamName) == 0 {
				w.Reply(":warning: The team name must be specified", replyInThread)
				return
			}
			description := req.StringParam("description", "")
			if len(description) == 0 {
				w.Reply(":warning: Description must be specified", replyInThread)
				return
			}

			idea, err := newIdeaFromDescription(description)
			if err != nil {
				w.Reply(fmt.Sprintf(":warning: %v", err), replyInThread)
				return
			}
			idea.Team = teamName
			idea.From = req.Event().User
			if err := c.addToList(context.TODO(), idea); err != nil {
				w.Reply(fmt.Sprintf(":warning: unable to persist idea: %v", err), replyInThread)
				return
			}
			w.Reply(fmt.Sprintf(":good_idea: idea was added to team %s list", teamName))
			return
		},
	})

	s.Command("ideas <team>", &slacker.CommandDefinition{
		Description: "List ideas recorded for the team.",
		Handler: func(req slacker.Request, w slacker.ResponseWriter) {
			teamName := req.StringParam("team", "")
			listMessage := []string{}
			curr := c.getList(context.TODO())
			if curr == nil {
				w.Reply(":sadpanda: no ideas recorded")
				return
			}
			for _, i := range curr.Items {
				if len(teamName) != 0 && i.Team != teamName {
					continue
				}
				// TODO: humanize time
				listMessage = append(listMessage, "> [%s] %s: %s (_%s_)", humanize.Time(i.Timestamp), i.From, i.Title, i.Clarification)
			}
			if len(listMessage) == 0 {
				w.Reply(":sadpanda: no ideas recorded")
				return
			}
			result := append([]string{"Ideas Recorded"}, listMessage...)
			w.Reply(fmt.Sprintf("%s", strings.Join(result, "\n")))
		},
	})

	s.Command("reset-ideas <team>", &slacker.CommandDefinition{
		Description: "Reset all ideas for the team.",
		Handler: func(req slacker.Request, w slacker.ResponseWriter) {
			teamName := req.StringParam("team", "")
			if err := c.resetListForTeam(context.TODO(), teamName); err != nil {
				w.Reply(fmt.Sprintf(":warning: unable to reset ideas for team %q: %v", teamName, err), replyInThread)
				return
			}
			w.Reply(fmt.Sprintf(":sweepclean: ideas for team %q successfully reset", teamName))
		},
	})
}
