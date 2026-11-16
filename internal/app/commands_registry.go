package app

import (
	"context"
	"errors"
	"io"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type commandEnv struct {
	ctx    context.Context
	cfg    *config.Config
	client *solutions.Client
	ws     *workspace.Workspace
	in     io.Reader
	out    io.Writer
}

type commandSpec struct {
	name           string
	needsWorkspace bool
	needsLogin     bool
	mutates        func([]string) bool
	run            func(commandEnv, []string) error
}

func alwaysMutates([]string) bool { return true }

func neverMutates([]string) bool { return false }

func filesMutates(args []string) bool {
	return len(args) > 0 && args[0] == "upload"
}

func catalogMutates(args []string) bool {
	return len(args) == 0 || args[0] != "list"
}

var commandRegistry = map[string]commandSpec{
	"login": {
		name: "login",
		run: func(env commandEnv, args []string) error {
			return login(env.ctx, env.cfg, env.client, args, env.in, env.out)
		},
	},
	"logout": {
		name: "logout",
		run: func(env commandEnv, args []string) error {
			return logout(env.cfg, args, env.out)
		},
	},
	"config": {
		name: "config",
		run: func(env commandEnv, args []string) error {
			return configCommand(env.ctx, env.cfg, env.client, args, env.out)
		},
	},
	"completion": {
		name: "completion",
		run: func(env commandEnv, args []string) error {
			return completionCommand(args, env.out)
		},
	},
	"path": {
		name: "path",
		run: func(env commandEnv, args []string) error {
			if len(args) != 0 {
				return errors.New("usage: panda path")
			}
			return printWorkspacePath(env.cfg, env.out)
		},
	},
	"doctor": {
		name: "doctor",
		run: func(env commandEnv, args []string) error {
			return doctorCommand(args, env.out)
		},
	},
	"browse": {
		name:           "browse",
		needsWorkspace: true,
		mutates:        neverMutates,
		run: func(env commandEnv, args []string) error {
			return browseTasks(env.ws, args, env.cfg.Email, env.cfg.APIURL, env.in, env.out)
		},
	},
	"list": {
		name:           "list",
		needsWorkspace: true,
		mutates:        neverMutates,
		run: func(env commandEnv, args []string) error {
			return listTasksForUser(env.ws, args, env.cfg.Email, env.out)
		},
	},
	"estimate": {
		name:           "estimate",
		needsWorkspace: true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			return estimateTasks(env.ws, args, env.in, env.out)
		},
	},
	"pull": {
		name:           "pull",
		needsWorkspace: true,
		needsLogin:     true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			if len(args) != 0 {
				return errors.New("usage: panda pull")
			}
			return pull(env.ctx, env.client, env.ws, env.cfg.Project, env.out)
		},
	},
	"diff": {
		name:           "diff",
		needsWorkspace: true,
		mutates:        neverMutates,
		run: func(env commandEnv, args []string) error {
			return diff(env.ws, args, env.out)
		},
	},
	"push": {
		name:           "push",
		needsWorkspace: true,
		needsLogin:     true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			options, err := parsePushOptions(args)
			if err != nil {
				return err
			}
			return pushSelectedWithOptions(env.ctx, env.client, env.ws, env.cfg.Project, options, env.out)
		},
	},
	"create": {
		name:           "create",
		needsWorkspace: true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			return createTask(env.ws, env.cfg.Project, args, env.in, env.out)
		},
	},
	"edit": {
		name:           "edit",
		needsWorkspace: true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			return editTask(env.ws, args, env.in, env.out)
		},
	},
	"assign": {
		name:           "assign",
		needsWorkspace: true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			return assignTask(env.ws, args, env.in, env.out)
		},
	},
	"link": {
		name:           "link",
		needsWorkspace: true,
		mutates:        neverMutates,
		run: func(env commandEnv, args []string) error {
			return linkTaskWithAPIURL(env.ws, args, env.cfg.APIURL, env.out)
		},
	},
	"edit-note": {
		name:           "edit-note",
		needsWorkspace: true,
		needsLogin:     true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			return editNoteContext(env.ctx, env.ws, args, env.client, env.in, env.out)
		},
	},
	"view": {
		name:           "view",
		needsWorkspace: true,
		mutates:        neverMutates,
		run: func(env commandEnv, args []string) error {
			return viewTaskWithAPIURL(env.ws, args, env.cfg.APIURL, env.out)
		},
	},
	"files": {
		name:           "files",
		needsWorkspace: true,
		mutates:        filesMutates,
		run: func(env commandEnv, args []string) error {
			return filesCommandContext(env.ctx, env.ws, args, env.client, env.out)
		},
	},
	"add-note": {
		name:           "add-note",
		needsWorkspace: true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			return addNote(env.ws, args, env.in, env.out)
		},
	},
	"discard": {
		name:           "discard",
		needsWorkspace: true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			return discard(env.ws, args, env.out)
		},
	},
	"clean": {
		name:           "clean",
		needsWorkspace: true,
		mutates:        alwaysMutates,
		run: func(env commandEnv, args []string) error {
			return clean(env.ws, args, env.in, env.out)
		},
	},
	"labels": {
		name:           "labels",
		needsWorkspace: true,
		mutates:        catalogMutates,
		run: func(env commandEnv, args []string) error {
			return projectCatalogCommand(env.ws, env.cfg.Project, labelCatalog, args, env.in, env.out)
		},
	},
	"packages": {
		name:           "packages",
		needsWorkspace: true,
		mutates:        catalogMutates,
		run: func(env commandEnv, args []string) error {
			return projectCatalogCommand(env.ws, env.cfg.Project, packageCatalog, args, env.in, env.out)
		},
	},
}
