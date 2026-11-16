package app

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func Run(args []string, in io.Reader, out, errOut io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), cancellationSignals()...)
	defer stop()
	return RunContext(ctx, args, in, out, errOut)
}

func RunContext(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
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
	if args[0] == "doctor" {
		return doctorCommand(args[1:], out)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	client := solutions.NewClient(cfg.APIURL, cfg.Token)
	client.Email = cfg.Email
	command, args := args[0], args[1:]
	spec, ok := commandRegistry[command]
	if !ok {
		return unknownCommandError(command)
	}
	env := commandEnv{ctx: ctx, cfg: cfg, client: client, in: in, out: out}
	if !spec.needsWorkspace {
		return spec.run(env, args)
	}
	var ws *workspace.Workspace
	if spec.readOnly {
		ws, err = readOnlyProjectWorkspace(cfg)
	} else if spec.mutates != nil && spec.mutates(args) {
		var lock *workspace.Lock
		ws, lock, err = lockProjectWorkspace(cfg)
		if err != nil {
			return err
		}
		defer lock.Close()
	} else {
		ws, err = projectWorkspace(cfg)
	}
	if err != nil {
		return err
	}
	env.ws = ws
	if spec.needsLogin {
		if err := requireLogin(cfg); err != nil {
			return err
		}
	}
	return spec.run(env, args)
}
