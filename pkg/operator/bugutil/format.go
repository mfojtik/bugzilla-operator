package bugutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

func GetBugURL(b bugzilla.Bug) string {
	return fmt.Sprintf("<https://bugzilla.redhat.com/show_bug.cgi?id=%d|#%d>", b.ID, b.ID)
}

// ParseLastChangeTime parse the "2020-05-20 10:45:16 +0000 UTC" to "2020-05-20T10:45:16Z" which can be used for cache revision.
func LastChangeTimeToRevision(value string) string {
	parsedTime, err := time.Parse("2006-01-02 15:04:05 -0700 MST", value)
	if err != nil {
		return value
	}
	return parsedTime.Format("2006-01-02T15:04:05Z")
}

func BugCountPlural(c int, capitalize bool) string {
	out := "bugs"
	if c == 0 {
		out = "no bugs"
	}
	if c == 1 {
		out = "bug"
	}
	if capitalize {
		return strings.Title(fmt.Sprintf("%d %s", c, out))
	}
	return fmt.Sprintf("%d %s", c, out)
}

func FormatBugMessage(b bugzilla.Bug) string {
	format := func(priority string) string {
		switch priority {
		case "urgent":
			return ":warning:*urgent*"
		case "high":
			return "*high*"
		case "low":
			return "low"
		default:
			return "_" + priority + "_"
		}
	}
	prio := ""
	if len(b.Priority) > 0 && len(b.Severity) > 0 {
		prio = fmt.Sprintf(" (%s/%s)", format(b.Severity), format(b.Priority))
	}
	return fmt.Sprintf("> %s [*%s*] %s%s", GetBugURL(b), b.Status, b.Summary, prio)
}

// DegradePriority transition Priority and Severity fields one level down
func DegradePriority(trans []config.Transition, in string) string {
	for _, t := range trans {
		if t.From == in {
			return t.To
		}
	}
	return ""
}
