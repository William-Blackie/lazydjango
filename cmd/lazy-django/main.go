package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/awesome-gocui/gocui"
	"github.com/williamblackie/lazydjango/pkg/django"
	"github.com/williamblackie/lazydjango/pkg/gui"
)

var errShowHelp = errors.New("show help")

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "source"
)

type cliOptions struct {
	doctor       bool
	doctorStrict bool
	doctorJSON   bool
	projectDir   string
	showVersion  bool
}

func usage() string {
	return `Usage: lazy-django [options]

Options:
  --doctor         Run dependency preflight checks and exit
  --doctor-strict  Exit non-zero if required dependencies are missing (with --doctor)
  --doctor-json    Emit doctor output as JSON (with --doctor)
  --project <dir>  Project directory to inspect (default: current directory)
  -v, --version    Show version
  -h, --help       Show help
`
}

func versionString() string {
	return fmt.Sprintf("lazy-django %s (commit=%s date=%s builtBy=%s)", version, commit, date, builtBy)
}

func parseOptions(args []string) (cliOptions, error) {
	var opts cliOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--doctor":
			opts.doctor = true
		case "--doctor-strict":
			opts.doctorStrict = true
		case "--doctor-json":
			opts.doctorJSON = true
		case "--project":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--project requires a directory")
			}
			i++
			opts.projectDir = args[i]
		case "-v", "--version":
			opts.showVersion = true
		case "-h", "--help":
			return opts, errShowHelp
		default:
			if strings.HasPrefix(arg, "--project=") {
				opts.projectDir = strings.TrimPrefix(arg, "--project=")
				if opts.projectDir == "" {
					return opts, fmt.Errorf("--project requires a directory")
				}
				continue
			}
			return opts, fmt.Errorf("unknown option: %s", arg)
		}
	}

	if (opts.doctorStrict || opts.doctorJSON) && !opts.doctor {
		return opts, fmt.Errorf("--doctor-strict/--doctor-json require --doctor")
	}

	return opts, nil
}

func main() {
	debug := os.Getenv("DEBUG") != ""
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		if errors.Is(err, errShowHelp) {
			fmt.Fprint(os.Stdout, usage())
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n\n%s", err, usage())
		os.Exit(2)
	}

	if opts.showVersion {
		fmt.Fprintln(os.Stdout, versionString())
		return
	}

	startDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if opts.projectDir != "" {
		startDir, err = filepath.Abs(opts.projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving project path: %v\n", err)
			os.Exit(1)
		}
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Working directory: %s\n", startDir)
	}

	discoveryOpts := django.DiscoverOptions{
		// Keep normal GUI startup fast; the GUI hydrates deep metadata asynchronously.
		DeepScan: opts.doctor,
	}
	project, err := django.DiscoverProjectWithOptions(startDir, discoveryOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Make sure you're in a Django project directory (with manage.py)")
		os.Exit(1)
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Project discovered: %s\n", project.RootDir)
		fmt.Fprintf(os.Stderr, "[DEBUG] Apps: %d, Models: %d\n", len(project.Apps), len(project.Models))
		fmt.Fprintf(os.Stderr, "[DEBUG] Database: %s\n", project.Database.Engine)
	}

	if opts.doctor {
		report := django.BuildDependencyReport(project)
		if opts.doctorJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing doctor JSON: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Fprint(os.Stdout, report.String())
		}
		if opts.doctorStrict && !report.IsHealthy() {
			os.Exit(2)
		}
		return
	}

	// Run GUI
	if err := gui.RunWithVersion(project, version); err != nil {
		if errors.Is(err, gocui.ErrQuit) {
			return
		}
		fmt.Fprintf(os.Stderr, "GUI Error: %v\n", err)
		os.Exit(1)
	}
}
