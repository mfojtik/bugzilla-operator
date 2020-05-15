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
			return "unknown"
		}
	}
	return fmt.Sprintf("> <https://bugzilla.redhat.com/show_bug.cgi?id=%d|#%d> [*%s*] %s (_%s/%s_)", b.ID, b.ID, b.Status, b.Summary, format(b.Priority), format(b.Severity))
}
