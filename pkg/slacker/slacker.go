package slacker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/gorilla/handlers"
	"github.com/shomali11/proper"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog"
)

const (
	space               = " "
	dash                = "-"
	star                = "*"
	newLine             = "\n"
	helpCommand         = "help"
	codeMessageFormat   = "`%s`"
	boldMessageFormat   = "*%s*"
	italicMessageFormat = "_%s_"
	quoteMessageFormat  = ">_*Example:* %s_"
	authorizedUsersOnly = "Authorized users only"
)

type Slacker struct {
	listenAddress     string
	verificationToken string
	client            *slack.Client

	botCommands           []BotCommand
	helpDefinition        *CommandDefinition
	defaultMessageHandler func(request Request, response ResponseWriter)

	linkSharedSubscribers    []func(*slackevents.LinkSharedEvent)
	blockActionSubscribers map[string]func(message *slack.Container, user *slack.User, action *slack.BlockAction) // by block ID
}

func NewSlacker(client *slack.Client, opt Options) *Slacker {
	return &Slacker{
		client:            client,
		listenAddress:     opt.ListenAddress,
		verificationToken: opt.VerificationToken,
	}
}

// Help handle the help message, it will use the default if not set
func (s *Slacker) Help(definition *CommandDefinition) {
	s.helpDefinition = definition
}

// Command define a new command and append it to the list of existing commands
func (s *Slacker) Command(usage string, definition *CommandDefinition) {
	s.botCommands = append(s.botCommands, NewBotCommand(usage, definition))
}

// DefaultCommand handle messages when none of the commands are matched
func (s *Slacker) DefaultCommand(defaultMessageHandler func(request Request, response ResponseWriter)) {
	s.defaultMessageHandler = defaultMessageHandler
}

func (s *Slacker) Run(ctx context.Context) {
	for {
		if err := s.Listen(ctx); err != nil && !isRetriable(err) {
			klog.Fatal(err)
		}
		time.Sleep(5 * time.Second)
	}
}

func (s *Slacker) Listen(ctx context.Context) error {
	defer runtime.HandleCrash()
	s.appendHelpHandle()

	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		body := buf.String()
		eventsAPIEvent, e := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionVerifyToken(&slackevents.TokenComparator{VerificationToken: s.verificationToken}))
		if e != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}

		switch eventsAPIEvent.Type {
		case slackevents.URLVerification:
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal([]byte(body), &r)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
			w.Header().Set("Content-Type", "text")
			w.Write([]byte(r.Challenge))

		case slackevents.CallbackEvent:
			var _ CommandDefinition
			innerEvent := eventsAPIEvent.InnerEvent
			klog.Infof("CallbackEvent: %s", innerEvent.Type)

			switch ev := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				// fake message event
				msgEv := &slackevents.MessageEvent{
					Type:            ev.Type,
					User:            ev.User,
					Text:            ev.Text,
					TimeStamp:       ev.TimeStamp,
					ThreadTimeStamp: ev.ThreadTimeStamp,
					Channel:         ev.Channel,
					EventTimeStamp:  ev.EventTimeStamp,
					UserTeam:        ev.UserTeam,
					SourceTeam:      ev.SourceTeam,
				}

				// ignore my own messages
				if len(ev.BotID) > 0 {
					klog.Infof("Ignoring AppMentionEvent for bot")
					break
				}

				go s.handleMessage(ctx, s.client, msgEv)
			case *slackevents.LinkSharedEvent:
				for _, l := range ev.Links {
					klog.Infof("Received link: %s", l.URL)
				}
				for _, s := range s.linkSharedSubscribers {
					go s(ev)
				}
			}
		}
	})
	mux.HandleFunc("/interactivity", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.Write([]byte(fmt.Sprintf("parse error: %v", err)))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		payload := r.PostForm.Get("payload")
		if len(payload) == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}

		klog.Infof("InteractionCallback: %s", payload)

		var cb slack.InteractionCallback
		if err := json.Unmarshal([]byte(payload), &cb); err != nil {
			w.Write([]byte(fmt.Sprintf("JSON parse error: %v", err)))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if cb.Token != s.verificationToken {
			w.Write([]byte("Permission denied."))
			w.WriteHeader(http.StatusForbidden)
			return
		}

		switch cb.Type {
		case "block_actions":
			for _, a := range cb.ActionCallback.BlockActions {
				if handler := s.blockActionSubscribers[a.BlockID]; handler != nil {
					handler(&cb.Container, &cb.User, a)
				}
			}
		}

		w.WriteHeader(http.StatusOK)
	})

	klog.Infof("bot up and listening to slack on %s", s.listenAddress)
	server := &http.Server{Addr: s.listenAddress, Handler: handlers.LoggingHandler(os.Stdout, mux)}
	go func() {
		<-ctx.Done()
		klog.Infof("Shutting down")
		server.Close()
	}()
	return server.ListenAndServe()
}

func (s *Slacker) handleMessage(ctx context.Context, client *slack.Client, message *slackevents.MessageEvent) {
	response := NewResponse(message, client)

	for _, cmd := range s.botCommands {
		parameters, isMatch := cmd.Match(unescape(message.Text))
		if !isMatch {
			continue
		}

		if userInfo, err := s.client.GetUserInfo(message.User); err == nil {
			message.Username = userInfo.Name
		} else {
			klog.Warningf("Unable to get user %q info: %v", message.User, err)
		}

		request := NewRequest(ctx, message, parameters)
		if cmd.Definition().AuthorizationFunc != nil && !cmd.Definition().AuthorizationFunc(request) {
			response.ReportError(errors.New("You are not authorized to execute this command"))
			return
		}

		cmd.Execute(request, response)
		return
	}

	if s.defaultMessageHandler != nil {
		request := NewRequest(ctx, message, &proper.Properties{})
		s.defaultMessageHandler(request, response)
	}
}

func (s *Slacker) SubscribeLinkShared(f func(ev *slackevents.LinkSharedEvent)) error {
	s.linkSharedSubscribers = append(s.linkSharedSubscribers, f)
	return nil
}

func (s *Slacker) SubscribeBlockAction(blockId string, f func(message *slack.Container, user *slack.User, action *slack.BlockAction)) error {
	s.blockActionSubscribers[blockId] = f
	return nil
}

func unescape(input string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, input)
}

func (s *Slacker) appendHelpHandle() {
	if s.helpDefinition == nil {
		s.helpDefinition = &CommandDefinition{}
	}

	if s.helpDefinition.Handler == nil {
		s.helpDefinition.Handler = s.defaultHelp
	}

	if len(s.helpDefinition.Description) == 0 {
		s.helpDefinition.Description = helpCommand
	}

	s.botCommands = append(s.botCommands, NewBotCommand(helpCommand, s.helpDefinition))
}

func (s *Slacker) defaultHelp(request Request, response ResponseWriter) {
	authorizedCommandAvailable := false
	helpMessage := empty
	for _, command := range s.botCommands {
		tokens := command.Tokenize()
		for _, token := range tokens {
			if token.IsParameter() {
				helpMessage += fmt.Sprintf(codeMessageFormat, token.Word) + space
			} else {
				helpMessage += fmt.Sprintf(boldMessageFormat, token.Word) + space
			}
		}

		if len(command.Definition().Description) > 0 {
			helpMessage += dash + space + fmt.Sprintf(italicMessageFormat, command.Definition().Description)
		}

		if command.Definition().AuthorizationFunc != nil {
			authorizedCommandAvailable = true
			helpMessage += space + fmt.Sprintf(codeMessageFormat, star)
		}

		helpMessage += newLine

		if len(command.Definition().Example) > 0 {
			helpMessage += fmt.Sprintf(quoteMessageFormat, command.Definition().Example) + newLine
		}
	}

	if authorizedCommandAvailable {
		helpMessage += fmt.Sprintf(codeMessageFormat, star+space+authorizedUsersOnly) + newLine
	}
	response.Reply(helpMessage)
}

func isRetriable(err error) bool {
	// there are several conditions that result from closing the connection on our side
	switch {
	case err == nil,
		err == io.EOF,
		strings.Contains(err.Error(), "use of closed network connection"):
		return true
	case strings.Contains(err.Error(), "cannot unmarshal object into Go struct field"):
		// this could be a legitimate error, so log it to ensure we can debug
		klog.Infof("warning: Ignoring serialization error and continuing: %v", err)
		return true
	default:
		return false
	}
}
