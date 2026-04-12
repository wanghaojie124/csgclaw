package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	appversion "csgclaw/internal/version"
)

type App struct {
	stdout     io.Writer
	stderr     io.Writer
	httpClient HTTPClient
	serveFunc  func(context.Context, []string, GlobalOptions) error
	stopFunc   func([]string, GlobalOptions) error
	agentFunc  func(context.Context, []string, GlobalOptions) error
	botFunc    func(context.Context, []string, GlobalOptions) error
	roomFunc   func(context.Context, []string, GlobalOptions) error
	memberFunc func(context.Context, []string, GlobalOptions) error
	userFunc   func(context.Context, []string, GlobalOptions) error
}

type GlobalOptions struct {
	Endpoint string
	Token    string
	Output   string
	Config   string
	Version  bool
}

func New() *App {
	return &App{
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		httpClient: &http.Client{},
	}
}

func (a *App) Execute(ctx context.Context, args []string) error {
	globals, rest, err := a.parseGlobalOptions(args)
	if err != nil {
		return err
	}
	if globals.Version {
		a.printVersion()
		return nil
	}
	if len(rest) == 0 {
		a.usage()
		return flag.ErrHelp
	}

	switch rest[0] {
	case "onboard":
		return a.runOnboard(rest[1:], globals)
	case "serve":
		if a.serveFunc != nil {
			return a.serveFunc(ctx, rest[1:], globals)
		}
		return a.runServe(ctx, rest[1:], globals)
	case "stop":
		if a.stopFunc != nil {
			return a.stopFunc(rest[1:], globals)
		}
		return a.runStop(rest[1:], globals)
	case "agent":
		if a.agentFunc != nil {
			return a.agentFunc(ctx, rest[1:], globals)
		}
		return a.runAgent(ctx, rest[1:], globals)
	case "bot":
		if a.botFunc != nil {
			return a.botFunc(ctx, rest[1:], globals)
		}
		return a.runBot(ctx, rest[1:], globals)
	case "room":
		if a.roomFunc != nil {
			return a.roomFunc(ctx, rest[1:], globals)
		}
		return a.runRoom(ctx, rest[1:], globals)
	case "member":
		if a.memberFunc != nil {
			return a.memberFunc(ctx, rest[1:], globals)
		}
		return a.runMember(ctx, rest[1:], globals)
	case "user":
		if a.userFunc != nil {
			return a.userFunc(ctx, rest[1:], globals)
		}
		return a.runUser(ctx, rest[1:], globals)
	case "_serve":
		return a.runInternalServe(ctx, rest[1:], globals)
	default:
		a.usage()
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func (a *App) parseGlobalOptions(args []string) (GlobalOptions, []string, error) {
	fs := flag.NewFlagSet("csgclaw", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.Usage = a.usage

	opts := GlobalOptions{}
	fs.StringVar(&opts.Endpoint, "endpoint", "", "HTTP server endpoint")
	fs.StringVar(&opts.Token, "token", "", "API authentication token")
	fs.StringVar(&opts.Output, "output", "table", "output format: table or json")
	fs.StringVar(&opts.Config, "config", "", "path to config file")
	fs.BoolVar(&opts.Version, "version", false, "print version and exit")
	fs.BoolVar(&opts.Version, "V", false, "print version and exit")

	globalArgs, rest := splitLeadingFlags(args)
	if err := fs.Parse(globalArgs); err != nil {
		return GlobalOptions{}, nil, err
	}
	if opts.Output == "" {
		opts.Output = "table"
	}

	return opts, rest, nil
}

func splitLeadingFlags(args []string) ([]string, []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return args[:i], args[i+1:]
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return args[:i], args[i:]
		}
		if consumesValue(arg) && !strings.Contains(arg, "=") && i+1 < len(args) {
			i++
		}
	}
	return args, nil
}

func consumesValue(arg string) bool {
	switch arg {
	case "--endpoint", "--token", "--output", "--config":
		return true
	default:
		return false
	}
}

func (a *App) usage() {
	fmt.Fprintln(a.stderr, "csgclaw manages local CSGClaw agents.")
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Usage:")
	fmt.Fprintln(a.stderr, "  csgclaw [global-flags] <command> [args]")
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Available Commands:")
	fmt.Fprintln(a.stderr, "  onboard  Initialize local config and bootstrap state")
	fmt.Fprintln(a.stderr, "  serve    Start the local HTTP server")
	fmt.Fprintln(a.stderr, "  stop     Stop the local HTTP server")
	fmt.Fprintln(a.stderr, "  agent    Manage agents")
	fmt.Fprintln(a.stderr, "  bot      Manage bots")
	fmt.Fprintln(a.stderr, "  room     Manage IM rooms")
	fmt.Fprintln(a.stderr, "  member   Manage IM room members")
	fmt.Fprintln(a.stderr, "  user     Manage IM users")
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Examples:")
	fmt.Fprintln(a.stderr, "  csgclaw -h")
	fmt.Fprintln(a.stderr, "  csgclaw --version")
	fmt.Fprintln(a.stderr, "  csgclaw serve -h")
	fmt.Fprintln(a.stderr, "  csgclaw agent -h")
	fmt.Fprintln(a.stderr, "  csgclaw agent create -h")
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Global flags:")
	fmt.Fprintln(a.stderr, "  --endpoint string   HTTP server endpoint")
	fmt.Fprintln(a.stderr, "  --token string      API authentication token")
	fmt.Fprintln(a.stderr, "  --output string     Output format: table or json")
	fmt.Fprintln(a.stderr, "  --config string     Path to config file")
	fmt.Fprintln(a.stderr, "  --version, -V       Print version and exit")
}

func (a *App) printVersion() {
	fmt.Fprintf(a.stdout, "csgclaw version %s\n", appversion.Current())
}

func (a *App) usageCommandGroup(command string, summary string, usageLine string, subcommands []string) {
	fmt.Fprintf(a.stderr, "%s\n\n", summary)
	fmt.Fprintln(a.stderr, "Usage:")
	fmt.Fprintf(a.stderr, "  %s\n\n", usageLine)
	fmt.Fprintln(a.stderr, "Available Subcommands:")
	for _, line := range subcommands {
		fmt.Fprintf(a.stderr, "  %s\n", line)
	}
	fmt.Fprintln(a.stderr)
	fmt.Fprintf(a.stderr, "Run `csgclaw %s <subcommand> -h` for subcommand details.\n", command)
}

func (a *App) newCommandFlagSet(name string, usageLine string, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.Usage = func() {
		if summary != "" {
			fmt.Fprintf(a.stderr, "%s\n\n", summary)
		}
		fmt.Fprintln(a.stderr, "Usage:")
		fmt.Fprintf(a.stderr, "  %s\n", usageLine)
		if hasFlags(fs) {
			fmt.Fprintln(a.stderr)
			fmt.Fprintln(a.stderr, "Flags:")
			fs.PrintDefaults()
		}
	}
	return fs
}

func hasFlags(fs *flag.FlagSet) bool {
	hasAny := false
	fs.VisitAll(func(*flag.Flag) {
		hasAny = true
	})
	return hasAny
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}
