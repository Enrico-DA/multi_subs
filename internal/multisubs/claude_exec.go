package multisubs

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
	Score  float64
	OrgID  string
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
	for _, name := range sortedClaudeProfileNames(cfg) {
		if err := store.EnsureProfileReady(cfg.Profiles[name]); err != nil {
			return fmt.Errorf("Claude profile %q is unsafe: %w", name, err)
		}
	}
	fableRequested := claudeArgsRequestFable(args)

	eligible := make([]claudeExecCandidate, 0, len(cfg.Profiles)+1)
	eligibleOrganizations := make(map[string]struct{}, len(cfg.Profiles)+1)
	defaultAuthCtx, defaultAuthCancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
	defaultAuth, defaultAuthProbeErr := fetchClaudeAuthStatus(defaultAuthCtx, a.claudeCommandRunner(), "")
	defaultAuthCancel()
	if defaultAuthProbeErr == nil && validateClaudeRoutingAuth(defaultAuth) == nil {
		defaultUsageCtx, defaultUsageCancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
		defaultUsage, defaultUsageErr := fetchClaudeUsage(defaultUsageCtx, a.claudeCommandRunner(), "")
		defaultUsageCancel()
		if defaultUsageErr == nil && claudeUsageIsEligible(defaultUsage, fableRequested) {
			eligible = append(eligible, claudeExecCandidate{
				Target: claudeTarget{Name: claudeDefaultTarget, Kind: "default"},
				Score:  claudeUsageWorstPercent(defaultUsage, fableRequested),
				OrgID:  defaultAuth.OrgID,
			})
			eligibleOrganizations[defaultAuth.OrgID] = struct{}{}
		}
	}

	for _, name := range sortedClaudeProfileNames(cfg) {
		profile := cfg.Profiles[name]
		authCtx, authCancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
		auth, authErr := fetchClaudeAuthStatus(authCtx, a.claudeCommandRunner(), profile.ConfigDir)
		authCancel()
		if authErr != nil || validateClaudeRoutingAuth(auth) != nil {
			continue
		}
		if _, duplicate := eligibleOrganizations[auth.OrgID]; duplicate {
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
			Score:  claudeUsageWorstPercent(usage, fableRequested),
			OrgID:  auth.OrgID,
		})
		eligibleOrganizations[auth.OrgID] = struct{}{}
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Score != eligible[j].Score {
			return eligible[i].Score < eligible[j].Score
		}
		return eligible[i].Target.Name < eligible[j].Target.Name
	})

	for _, candidate := range eligible {
		reservation, acquired, err := store.acquireReservation(claudeReservationTargetForOrg(candidate.OrgID))
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		runErr := func() error {
			defer reservation.Release()
			return a.claudeCommandRunner().RunWithReservation(
				context.Background(),
				append([]string{"-p"}, args...),
				claudeEnv(os.Environ(), candidate.Target.ConfigDir),
				reservation.file,
			)
		}()
		return claudeChildError(runErr, "Claude command failed")
	}
	if len(eligible) > 0 {
		return &ExitError{Code: claudeBusyExitCode, Message: "all quota-eligible Claude accounts are busy"}
	}
	return &ExitError{Code: 1, Message: "no usable Claude account"}
}

func claudeArgsRequestFable(args []string) bool {
	model := ""
	fallbackModel := ""
	modelWasExplicit := false
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "--" {
			break
		}
		switch {
		case arg == "--model" || arg == "-m":
			if index+1 < len(args) {
				model = strings.TrimSpace(args[index+1])
				modelWasExplicit = true
				index++
			}
		case strings.HasPrefix(arg, "--model="):
			model = strings.TrimSpace(strings.TrimPrefix(arg, "--model="))
			modelWasExplicit = true
		case strings.HasPrefix(arg, "-m="):
			model = strings.TrimSpace(strings.TrimPrefix(arg, "-m="))
			modelWasExplicit = true
		case arg == "--fallback-model":
			if index+1 < len(args) {
				fallbackModel = strings.TrimSpace(args[index+1])
				index++
			}
		case strings.HasPrefix(arg, "--fallback-model="):
			fallbackModel = strings.TrimSpace(strings.TrimPrefix(arg, "--fallback-model="))
		}
	}
	if fallbackModel != "" && claudeModelMayUseFable(fallbackModel) {
		return true
	}
	if modelWasExplicit {
		return claudeModelMayUseFable(model)
	}
	if envModel := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL")); envModel != "" {
		return claudeModelMayUseFable(envModel)
	}
	// Without an explicit effective model, route conservatively because user or
	// managed settings may select Fable.
	return true
}

func claudeModelMayUseFable(models string) bool {
	for _, model := range strings.Split(models, ",") {
		lower := strings.ToLower(strings.TrimSpace(model))
		if lower == "" {
			return true
		}
		if strings.Contains(lower, "fable") {
			return true
		}
		if strings.Contains(lower, "sonnet") || strings.Contains(lower, "haiku") || strings.Contains(lower, "opus") {
			continue
		}
		return true
	}
	return false
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
