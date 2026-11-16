package app

import (
	"errors"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/report"
)

type pushOptions struct {
	selectors []string
	json      bool
}

type pushReport = report.PushReport

type pushSummary = report.PushSummary

func parsePushOptions(args []string) (pushOptions, error) {
	var options pushOptions
	for _, arg := range args {
		if arg == "--json" {
			options.json = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return pushOptions{}, errors.New("usage: panda push [taskNumber|draft-N ...] [--json]")
		}
		options.selectors = append(options.selectors, arg)
	}
	return options, nil
}
