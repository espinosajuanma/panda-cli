package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func printWorkspacePath(cfg *config.Config, out io.Writer) error {
	if cfg.Project == nil || cfg.Project.Name == "" {
		return errors.New("no project selected; run `panda config set project <name>` first")
	}
	path, err := workspace.WorkspacePath(cfg.Project.Name)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, path)
	return nil
}

func completionCommand(args []string, out io.Writer) error {
	if len(args) == 1 {
		switch args[0] {
		case "bash":
			fmt.Fprintln(out, renderCompletion(bashCompletion))
			return nil
		case "zsh":
			fmt.Fprintln(out, renderCompletion(zshCompletion))
			return nil
		case "fish":
			fmt.Fprintln(out, renderCompletion(fishCompletion))
			return nil
		}
	}
	return errors.New("usage: panda completion <bash|zsh|fish>")
}

func renderCompletion(template string) string {
	return strings.ReplaceAll(template, "@COMMANDS@", rootCommandWords())
}

const bashCompletion = `_panda_complete() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local candidate
  if [[ "${COMP_WORDS[1]}" == "edit" && $COMP_CWORD -eq 3 ]]; then
    COMPREPLY=( $(compgen -W "labels package assignees json" -- "$cur") )
  elif [[ "${COMP_WORDS[1]}" == "edit" && ( "${COMP_WORDS[3]}" == "labels" || "${COMP_WORDS[3]}" == "package" ) ]]; then
    COMPREPLY=( $(compgen -W "none" -- "$cur") )
  elif [[ ( "${COMP_WORDS[1]}" == "labels" || "${COMP_WORDS[1]}" == "packages" ) && $COMP_CWORD -eq 2 ]]; then
    COMPREPLY=( $(compgen -W "list create edit delete" -- "$cur") )
  else
    COMPREPLY=( $(compgen -W "@COMMANDS@" -- "$cur") )
  fi
}
complete -F _panda_complete panda`

const zshCompletion = `#compdef panda
_panda() {
  local cur="${words[CURRENT]}"
  if [[ "${words[2]}" == "edit" && $CURRENT -eq 4 ]]; then
    compadd -- labels package assignees json
  elif [[ ( "${words[2]}" == "labels" || "${words[2]}" == "packages" ) && $CURRENT -eq 3 ]]; then
    compadd -- list create edit delete
  else
    compadd -- @COMMANDS@
  fi
}
compdef _panda panda`

const fishCompletion = `complete -c panda -f -n '__fish_seen_subcommand_from edit; and test (count (commandline -opc)) -eq 3' -a 'labels package assignees json'
complete -c panda -f -n '__fish_seen_subcommand_from labels packages; and test (count (commandline -opc)) -eq 2' -a 'list create edit delete'
complete -c panda -f -n 'not __fish_seen_subcommand_from @COMMANDS@' -a '@COMMANDS@'`

func login(ctx context.Context, cfg *config.Config, client *solutions.Client, args []string, in io.Reader, out io.Writer) error {
	var email string
	if len(args) > 1 {
		return errors.New("usage: panda login [email]")
	}
	if len(args) == 1 {
		email = args[0]
	} else {
		value, err := prompt(in, out, "Email: ")
		if err != nil {
			return err
		}
		email = value
	}
	fmt.Fprint(out, "Password: ")
	var password string
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		data, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return err
		}
		password = string(data)
	} else {
		value, err := prompt(in, io.Discard, "")
		if err != nil {
			return err
		}
		password = value
	}
	token, err := client.Login(ctx, email, password)
	if err != nil {
		return err
	}
	cfg.Token = token
	cfg.Email = email
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}
	fmt.Fprintln(out, "Logged in successfully.")
	return nil
}

func configCommand(ctx context.Context, cfg *config.Config, client *solutions.Client, args []string, out io.Writer) error {
	if len(args) == 2 && args[0] == "get" && args[1] == "project" {
		if cfg.Project == nil {
			fmt.Fprintln(out, "No project selected.")
		} else {
			fmt.Fprintf(out, "%s (%s)\n", cfg.Project.Name, cfg.Project.ID)
		}
		return nil
	}
	if len(args) == 3 && args[0] == "set" && args[1] == "project" {
		if cfg.Token == "" {
			return errors.New("not logged in; run `panda login` first")
		}
		project, err := client.FindProject(ctx, args[2])
		if err != nil {
			return err
		}
		cfg.Project = &config.Project{ID: project.ID, Name: project.Name}
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Fprintf(out, "Selected project %s (%s).\n", project.Name, project.ID)
		return nil
	}
	return errors.New("usage: panda config get project | panda config set project <name>")
}
