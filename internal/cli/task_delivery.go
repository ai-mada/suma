package cli

import (
	"context"
	"io"
	"net/url"
	"regexp"
	"strings"
)

const taskDeliveryUsage = "task delivery requires one exact task ID; use task delivery TASK with --project"

var taskDeliveryID = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$")

func taskDeliveryCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") {
		_, err := io.WriteString(out, c.text(
			"  task delivery TASK   读取当前项目的任务交付证据（将 --project ID 放在命令前）\n",
			"  task delivery TASK   Read current task delivery evidence (put --project ID before the command).\n",
		))
		return err
	}
	if len(args) != 1 || !taskDeliveryID.MatchString(args[0]) || strings.Contains(args[0], "..") {
		return usage(taskDeliveryUsage)
	}
	return contextRead(ctx, c, "/api/v1/tasks/"+url.PathEscape(args[0])+"/delivery", out)
}
