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

// 2020-09-23T13:06:29Z

func ParseChangeWhenString(v string) time.Time {
	parsedTime, err := time.Parse("2006-01-02T15:04:05Z", v)
	if err != nil {
		return time.Time{}
	}
	return parsedTime
}

func ParseTimeString(v string) time.Time {
	parsedTime, err := time.Parse("2006-01-02 15:04:05 -0700 MST", v)
	if err != nil {
		return time.Time{}
	}
	return parsedTime
}

// ParseLastChangeTime parse the "2020-05-20 10:45:16 +0000 UTC" to "2020-05-20T10:45:16Z" which can be used for cache revision.
func LastChangeTimeToRevision(value string) string {
	parsedTime := ParseTimeString(value)
	if parsedTime.IsZero() {
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

func FormatPriority(priority string) string {
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

func FormatBugMessage(b bugzilla.Bug) string {
	prefix := ":bugzilla: "
	switch {
	case b.Severity == "urgent":
		prefix = ":red-siren: *URGENT*"
	}
	return fmt.Sprintf("%s %s [*%s*] %s – %s/%s in *%s* for *%s*/*%s*",
		prefix,
		GetBugURL(b),
		b.Status,
		b.Summary,
		FormatPriority(b.Severity),
		FormatPriority(b.Priority),
		FormatComponent(b.Component),
		FormatVersion(b.Version),
		FormatVersion(b.TargetRelease))
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

func FormatVersion(vs []string) string {
	if len(vs) > 0 {
		return vs[0]
	}
	return "---"
}

func FormatComponent(cs []string) string {
	if len(cs) == 1 {
		return cs[0]
	} else if len(cs) > 0 {
		return fmt.Sprintf("%s", cs)
	}
	return "---"
}
