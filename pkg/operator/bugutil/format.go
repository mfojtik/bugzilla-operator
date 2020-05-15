package bugutil

import (
	"fmt"

	"github.com/eparis/bugzilla"
)

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
	return fmt.Sprintf("> <https://bugzilla.redhat.com/show_bug.cgi?id=%d|#%d> [*%s*] %s%s", b.ID, b.ID, b.Status, b.Summary, prio)
}
