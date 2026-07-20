package multicodex

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

const claudeBusyExitCode = 75

type claudeExecCandidate struct {
	Target claudeTarget
	Usage  claudeUsage
	Score  float64
}

func (a *App) cmdClaudeExec(args []string) error {
	if commandArgsContainHelp(args) {
		err := a.claudeCommandRunner().Run(context.Background(), append([]string{"-p"}, args...), claudeEnv(os.Environ(), ""))
		return claudeChildError(err, "Claude help command failed")
	}

	store := newClaudeStore(a.store.paths)
	cfg, err := store.LoadIfExists()
	if err != nil {
		return err
	}
	fableRequested := claudeArgsRequestFable(args)
	eligible := make([]claudeExecCandidate, 0, len(cfg.Profiles))
	for _, name := range sortedClaudeProfileNames(cfg) {
		profile := cfg.Profiles[name]
		if err := store.EnsureProfileReady(profile); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
		usage, usageErr := fetchClaudeUsage(ctx, a.claudeCommandRunner(), profile.ConfigDir)
		cancel()
		if usageErr != nil || !claudeUsageIsEligible(usage, fableRequested) {
			continue
		}
		profileCopy := profile
		eligible = append(eligible, claudeExecCandidate{
			Target: claudeTarget{Name: name, Kind: "managed", ConfigDir: profile.ConfigDir, Profile: &profileCopy},
			Usage:  usage,
			Score:  claudeUsageWorstPercent(usage, fableRequested),
		})
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Score != eligible[j].Score {
			return eligible[i].Score < eligible[j].Score
		}
		return eligible[i].Target.Name < eligible[j].Target.Name
	})

	for _, candidate := range eligible {
		reservation, acquired, err := store.acquireReservation(candidate.Target.Name)
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		runErr := func() error {
			defer reservation.Release()
			return a.claudeCommandRunner().Run(
				context.Background(),
				append([]string{"-p"}, args...),
				claudeEnv(os.Environ(), candidate.Target.ConfigDir),
			)
		}()
		return claudeChildError(runErr, "Claude command failed")
	}
	if len(eligible) > 0 {
		return &ExitError{Code: claudeBusyExitCode, Message: "all quota-eligible Claude managed profiles are busy; the default reserve was not used"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
	defaultUsage, usageErr := fetchClaudeUsage(ctx, a.claudeCommandRunner(), "")
	cancel()
	if usageErr != nil {
		return &ExitError{Code: 1, Message: fmt.Sprintf("no usable Claude account: default reserve usage is unavailable (%s)", usageErr)}
	}
	if !claudeUsageIsEligible(defaultUsage, fableRequested) {
		return &ExitError{Code: 1, Message: "no quota-eligible Claude account; managed profiles were unavailable or exhausted and the default reserve is exhausted"}
	}
	reservation, acquired, err := store.acquireReservation(claudeDefaultTarget)
	if err != nil {
		return err
	}
	if !acquired {
		return &ExitError{Code: claudeBusyExitCode, Message: "Claude default reserve is busy"}
	}
	runErr := func() error {
		defer reservation.Release()
		return a.claudeCommandRunner().Run(
			context.Background(),
			append([]string{"-p"}, args...),
			claudeEnv(os.Environ(), ""),
		)
	}()
	return claudeChildError(runErr, "Claude command failed")
}

func claudeArgsRequestFable(args []string) bool {
	model := ""
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "--" {
			break
		}
		switch {
		case arg == "--model" || arg == "-m":
			if index+1 < len(args) {
				model = strings.TrimSpace(args[index+1])
				index++
			}
		case strings.HasPrefix(arg, "--model="):
			model = strings.TrimSpace(strings.TrimPrefix(arg, "--model="))
		case strings.HasPrefix(arg, "-m="):
			model = strings.TrimSpace(strings.TrimPrefix(arg, "-m="))
		}
	}
	return strings.Contains(strings.ToLower(model), "fable")
}

func claudeUsageIsEligible(usage claudeUsage, fableRequested bool) bool {
	if usage.Session.UsedPercent >= 100 || usage.WeeklyAll.UsedPercent >= 100 {
		return false
	}
	if !fableRequested {
		return true
	}
	return usage.Fable != nil && usage.Fable.UsedPercent < 100
}

func claudeUsageWorstPercent(usage claudeUsage, fableRequested bool) float64 {
	worst := math.Max(usage.Session.UsedPercent, usage.WeeklyAll.UsedPercent)
	if fableRequested && usage.Fable != nil {
		worst = math.Max(worst, usage.Fable.UsedPercent)
	}
	return worst
}
