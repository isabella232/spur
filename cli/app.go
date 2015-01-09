package cli

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"text/tabwriter"
	"text/template"
	"time"
)

// App is the main structure of a cli application. It is recomended that
// and app be created with the cli.NewApp() function
type App struct {
	// The name of the program. Defaults to os.Args[0]
	Name string
	// Description of the program.
	Usage string
	// Version of the program
	Version string
	// List of commands to execute
	Commands []Command
	// List of flags to parse
	Flags []Flag
	// Boolean to enable bash completion commands
	EnableBashCompletion bool
	// Boolean to hide built-in help command
	HideHelp bool
	// Boolean to hide built-in version flag
	HideVersion bool
	// An action to execute when the bash-completion flag is set
	BashComplete func(context *Context)
	// An action to execute before any subcommands are run, but after the context is ready
	// If a non-nil error is returned, no subcommands are run
	Before func(context *Context) error
	// An action to execute after any subcommands are run, but after the subcommand has finished
	// It is run regardless of Because() result
	After func(context *Context) error
	// The action to execute when no subcommands are specified
	Action func(context *Context)
	// Execute this function if the proper command cannot be found
	CommandNotFound func(context *Context, command string)
	// Compilation date
	Compiled time.Time
	// Author
	Author string
	// Author e-mail
	Email string
	// Writer writer to write output to
	Writer io.Writer
}

// Tries to find out when this binary was compiled.
// Returns the current time if it fails to find it.
func compileTime() time.Time {
	info, err := os.Stat(os.Args[0])
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}

// Creates a new cli Application with some reasonable defaults for Name, Usage, Version and Action.
func NewApp() *App {
	return &App{
		Name:         os.Args[0],
		Usage:        "A new cli application",
		Version:      "0.0.0",
		BashComplete: DefaultAppComplete,
		Action:       helpCommand.Action,
		Compiled:     compileTime(),
		Author:       "Author",
		Email:        "unknown@email",
		Writer:       os.Stdout,
	}
}

// Entry point to the cli app. Parses the arguments slice and routes to the proper flag/args combination
func (a *App) Run(arguments []string) error {
	var err error
	if HelpPrinter == nil {
		defer func() {
			HelpPrinter = nil
		}()

		HelpPrinter = func(templ string, data interface{}) {
			w := tabwriter.NewWriter(a.Writer, 0, 8, 1, '\t', 0)
			t := template.Must(template.New("help").Parse(templ))
			err := t.Execute(w, data)
			if err != nil {
				panic(err)
			}
			w.Flush()
		}
	}

	// append help to commands
	if a.Command(helpCommand.Name) == nil && !a.HideHelp {
		a.Commands = append(a.Commands, helpCommand)
		if (HelpFlag != BoolFlag{}) {
			a.appendFlag(HelpFlag)
		}
	}

	//append version/help flags
	if a.EnableBashCompletion {
		a.appendFlag(BashCompletionFlag)
	}

	if !a.HideVersion {
		a.appendFlag(VersionFlag)
	}

	// parse flags
	set := flagSet(a.Name, a.Flags)
	set.SetOutput(ioutil.Discard)
	err = set.Parse(arguments[1:])
	nerr := normalizeFlags(a.Flags, set)
	if nerr != nil {
		fmt.Fprintln(a.Writer, nerr)
		context := NewContext(a, set, set)
		ShowAppHelp(context)
		fmt.Fprintln(a.Writer)
		return nerr
	}
	context := NewContext(a, set, set)

	if err != nil {
		fmt.Fprintf(a.Writer, "Incorrect Usage.\n\n")
		ShowAppHelp(context)
		fmt.Fprintln(a.Writer)
		return err
	}

	if checkCompletions(context) {
		return nil
	}

	if checkHelp(context) {
		return nil
	}

	if checkVersion(context) {
		return nil
	}

	if a.After != nil {
		defer func() {
			// err is always nil here.
			// There is a check to see if it is non-nil
			// just few lines before.
			err = a.After(context)
		}()
	}

	if a.Before != nil {
		err := a.Before(context)
		if err != nil {
			return err
		}
	}

	args := context.Args()
	if args.Present() {
		name := args.First()
		c := a.Command(name)
		if c != nil {
			return c.Run(context)
		}
	}

	// Run default Action
	a.Action(context)
	return nil
}

// Another entry point to the cli app, takes care of passing arguments and error handling
func (a *App) RunAndExitOnError() {
	if err := a.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Invokes the subcommand given the context, parses ctx.Args() to generate command-specific flags
func (a *App) RunAsSubcommand(ctx *Context) error {
	var err error
	// append help to commands
	if len(a.Commands) > 0 {
		if a.Command(helpCommand.Name) == nil && !a.HideHelp {
			a.Commands = append(a.Commands, helpCommand)
			if (HelpFlag != BoolFlag{}) {
				a.appendFlag(HelpFlag)
			}
		}
	}

	// append flags
	if a.EnableBashCompletion {
		a.appendFlag(BashCompletionFlag)
	}

	// parse flags
	set := flagSet(a.Name, a.Flags)
	set.SetOutput(ioutil.Discard)
	err = set.Parse(ctx.Args().Tail())
	nerr := normalizeFlags(a.Flags, set)
	context := NewContext(a, set, ctx.globalSet)

	if nerr != nil {
		fmt.Fprintln(a.Writer, nerr)
		if len(a.Commands) > 0 {
			ShowSubcommandHelp(context)
		} else {
			ShowCommandHelp(ctx, context.Args().First())
		}
		fmt.Fprintln(a.Writer)
		return nerr
	}

	if err != nil {
		fmt.Fprintf(a.Writer, "Incorrect Usage.\n\n")
		ShowSubcommandHelp(context)
		return err
	}

	if checkCompletions(context) {
		return nil
	}

	if len(a.Commands) > 0 {
		if checkSubcommandHelp(context) {
			return nil
		}
	} else {
		if checkCommandHelp(ctx, context.Args().First()) {
			return nil
		}
	}

	if a.After != nil {
		defer func() {
			// err is always nil here.
			// There is a check to see if it is non-nil
			// just few lines before.
			err = a.After(context)
		}()
	}

	if a.Before != nil {
		err := a.Before(context)
		if err != nil {
			return err
		}
	}

	args := context.Args()
	if args.Present() {
		name := args.First()
		c := a.Command(name)
		if c != nil {
			return c.Run(context)
		}
	}

	// Run default Action
	if len(a.Commands) > 0 {
		a.Action(context)
	} else {
		a.Action(ctx)
	}

	return nil
}

// Returns the named command on App. Returns nil if the command does not exist
func (a *App) Command(name string) *Command {
	for _, c := range a.Commands {
		if c.HasName(name) {
			return &c
		}
	}

	return nil
}

func (a *App) hasFlag(flag Flag) bool {
	for _, f := range a.Flags {
		if flag == f {
			return true
		}
	}

	return false
}

func (a *App) appendFlag(flag Flag) {
	if !a.hasFlag(flag) {
		a.Flags = append(a.Flags, flag)
	}
}
