package editorleaf

import (
	"context"

	"github.com/ge-editor/gecore/excommand"
)

var exCommands = excommand.NewRegistry()

func RegisterGlobal(cmd excommand.ExCommand) {
	exCommands.Register(cmd)
}

// Implements

type Command struct {
	CommandName        string
	CommandAliases     []string
	CommandDescription string
	Run                func(ctx context.Context, args []string) (handled bool, err error)

	// Complete, if set, lists the candidates for the argument typed after
	// the command name (see excommand.ArgCompleter).
	Complete func(input string) []string
}

func (c *Command) Name() string        { return c.CommandName }
func (c *Command) Aliases() []string   { return c.CommandAliases }
func (c *Command) Description() string { return c.CommandDescription }

// CompleteArgs implements excommand.ArgCompleter.
func (c *Command) CompleteArgs(input string) []string {
	if c.Complete == nil {
		return nil
	}
	return c.Complete(input)
}

func (c *Command) Execute(ctx context.Context, args []string) (bool, error) {
	if c.Run == nil {
		return false, nil
	}
	return c.Run(ctx, args)
}

// -------------------

func (e *Editorleaf) Commands() []excommand.ExCommand {
	return []excommand.ExCommand{
		e.languageCommand(),

		e.shellCommand(),
		e.shellRegionCommand(),
	}
}
