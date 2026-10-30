package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
)

func Run(args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return printHelp(out)
	}
	if args[0] == "help" {
		commandArgs := args[1:]
		if len(commandArgs) > 0 && (commandArgs[len(commandArgs)-1] == "--help" || commandArgs[len(commandArgs)-1] == "-h") {
			commandArgs = commandArgs[:len(commandArgs)-1]
		}
		if len(commandArgs) > 0 {
			return printCommandHelp(strings.Join(commandArgs, " "), out)
		}
		return printHelp(out)
	}
	if len(args) > 1 && (args[len(args)-1] == "--help" || args[len(args)-1] == "-h") {
		return printCommandHelp(commandHelpKey(args[:len(args)-1]), out)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	client := solutions.NewClient(cfg.APIURL, cfg.Token)
	client.Email = cfg.Email
	command, args := args[0], args[1:]
	switch command {
	case "login":
		return login(context.Background(), cfg, client, args, in, out)
	case "config":
		return configCommand(context.Background(), cfg, client, args, out)
	case "completion":
		return completionCommand(args, out)
	case "path":
		if len(args) != 0 {
			return errors.New("usage: panda path")
		}
		return printWorkspacePath(cfg, out)
	default:
		if !commandNeedsWorkspace(command) {
			if err := printHelp(errOut); err != nil {
				return err
			}
			return fmt.Errorf("unknown command %q", command)
		}
		ws, err := projectWorkspace(cfg)
		if err != nil {
			return err
		}
		switch command {
		case "list":
			return listTasks(ws, args, out)
		case "estimate":
			return estimateTasks(ws, args, in, out)
		case "pull":
			if len(args) != 0 {
				return errors.New("usage: panda pull")
			}
			if err := requireLogin(cfg); err != nil {
				return err
			}
			return pull(context.Background(), client, ws, cfg.Project, out)
		case "diff":
			return diff(ws, args, out)
		case "push":
			if err := requireLogin(cfg); err != nil {
				return err
			}
			return pushSelected(context.Background(), client, ws, cfg.Project, args, out)
		case "create":
			return createTask(ws, cfg.Project, args, in, out)
		case "edit":
			return editTask(ws, args, in, out)
		case "assign":
			return assignTask(ws, args, out)
		case "link":
			return linkTask(ws, args, out)
		case "edit-note":
			if err := requireLogin(cfg); err != nil {
				return err
			}
			return editNote(ws, args, client, out)
		case "view":
			return viewTask(ws, args, out)
		case "files":
			return filesCommand(ws, args, client, out)
		case "add-note":
			return addNote(ws, args, in, out)
		case "discard":
			return discard(ws, args, out)
		case "clean":
			return clean(ws, args, in, out)
		case "labels":
			return projectCatalogCommand(ws, cfg.Project, labelCatalog, args, in, out)
		case "packages":
			return projectCatalogCommand(ws, cfg.Project, packageCatalog, args, in, out)
		}
	}
	return nil
}
