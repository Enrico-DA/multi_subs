package multisubs

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Enrico-DA/multi_subs/internal/monitor/usage"
)

func (a *App) cmdMonitor(args []string) error {
	if len(args) == 0 {
		printMonitorUsage()
		return nil
	}

	switch args[0] {
	case "doctor":
		return a.runMonitorDoctor(args[1:])
	case "completion":
		return a.runMonitorCompletion(args[1:])
	case "help", "-h", "--help":
		if err := rejectArguments(args[1:], "usage: multisubs codex monitor help"); err != nil {
			return err
		}
		printMonitorUsage()
		return nil
	default:
		return &ExitError{Code: 2, Message: fmt.Sprintf("unknown monitor command: %s\nrun \"multisubs help codex monitor\" for monitor usage", args[0])}
	}
}

func (a *App) runMonitorDoctor(args []string) error {
	fs := flag.NewFlagSet("monitor doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "output doctor report as JSON")
	timeout := fs.Duration("timeout", 60*time.Second, "doctor timeout")
	includeDefault := fs.Bool("include-default", true, "include the global Codex home")
	includeActive := fs.Bool("include-active", false, "include the active CODEX_HOME")
	discover := fs.Bool("discover", false, "discover compatible Codex homes from the filesystem")
	appServer := fs.Bool("app-server", false, "also check the raw Codex app-server source separately")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2, Message: "usage: multisubs codex monitor doctor [--json] [--timeout 60s] [--include-default] [--include-active] [--discover] [--app-server]"}
	}
	if fs.NArg() != 0 {
		return &ExitError{Code: 2, Message: "usage: multisubs codex monitor doctor [--json] [--timeout 60s] [--include-default] [--include-active] [--discover] [--app-server]"}
	}
	if *timeout <= 0 {
		return &ExitError{Code: 2, Message: "error: --timeout must be > 0"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	report := usage.RunDoctor(ctx, usage.DoctorOptions{
		Accounts: usage.MonitorAccountOptions{
			IncludeDefault: *includeDefault,
			IncludeActive:  *includeActive,
			Discover:       *discover,
		},
		IncludeAppServer: *appServer,
	})
	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printMonitorDoctorHuman(report)
	}
	return monitorDoctorResult(report)
}

func monitorDoctorResult(report usage.DoctorReport) error {
	if report.Healthy() {
		return nil
	}
	return &ExitError{Code: 1, Message: "monitor doctor checks failed"}
}

func (a *App) runMonitorCompletion(args []string) error {
	if len(args) > 1 {
		return &ExitError{Code: 2, Message: "usage: multisubs codex monitor completion [bash|zsh|fish]"}
	}

	shell := "bash"
	if len(args) == 1 {
		shell = strings.TrimSpace(args[0])
		if shell == "" {
			shell = "bash"
		}
	}

	return a.cmdCompletion([]string{shell})
}

func printMonitorDoctorHuman(report usage.DoctorReport) {
	printMonitorDoctorHumanTo(os.Stdout, report)
}

func printMonitorDoctorHumanTo(w io.Writer, report usage.DoctorReport) {
	fmt.Fprintln(w, "multisubs codex monitor doctor")
	fmt.Fprintln(w)
	for _, c := range report.Checks {
		state := "FAIL"
		if c.OK {
			state = "PASS"
		}
		fmt.Fprintf(w, "[%s] %s\n", state, c.Name)
		fmt.Fprintf(w, "  %s\n", c.Details)
	}
	fmt.Fprintln(w)
	switch report.Status() {
	case "healthy":
		fmt.Fprintln(w, "monitor doctor result: PASS")
	case "degraded":
		fmt.Fprintln(w, "monitor doctor result: FAIL (degraded: at least one check failed)")
	default:
		fmt.Fprintln(w, "monitor doctor result: FAIL")
	}
}

func printMonitorUsage() {
	fmt.Println("multisubs codex monitor")
	fmt.Println()
	fmt.Println("Check Codex subscription usage sources across multisubs profiles and compatible local accounts.")
	fmt.Println("The monitor is read-only and does not mutate Codex account data.")
	fmt.Println("For a usage snapshot, run \"multisubs usage\".")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  multisubs codex monitor doctor [flags]        Run setup and source checks")
	fmt.Println("  multisubs codex monitor completion [shell]    Print shell completion script")
	fmt.Println()
	fmt.Println("Shell completion:")
	fmt.Println("  multisubs completion bash")
	fmt.Println("  multisubs completion zsh")
	fmt.Println("  multisubs completion fish")
	fmt.Println("  multisubs codex monitor completion bash")
	fmt.Println()
	fmt.Println("Monitor doctor flags:")
	fmt.Println("  --json            Output report as JSON")
	fmt.Println("  --timeout 60s     Doctor timeout")
	fmt.Println("  --include-default Include the global Codex home (default true)")
	fmt.Println("  --include-active  Include the active CODEX_HOME")
	fmt.Println("  --discover        Discover compatible Codex homes from the filesystem")
	fmt.Println("  --app-server      Also check the raw Codex app-server source separately")
	fmt.Println()
}
