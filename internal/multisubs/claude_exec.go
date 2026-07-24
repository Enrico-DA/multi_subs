package multisubs

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
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
	fableResolver := a.claudeFableApplicabilityResolver()
	intent := fableResolver.ParseIntent(args, os.Environ())

	eligible := make([]claudeExecCandidate, 0, len(cfg.Profiles)+1)
	eligibleOrganizations := make(map[string]struct{}, len(cfg.Profiles)+1)
	defaultTarget := claudeTarget{Name: claudeDefaultTarget, Kind: "default"}
	defaultAuthCtx, defaultAuthCancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
	defaultAuth, defaultAuthProbeErr := fetchClaudeAuthStatus(defaultAuthCtx, a.claudeCommandRunner(), "")
	defaultAuthCancel()
	if defaultAuthProbeErr == nil && validateClaudeRoutingAuth(defaultAuth) == nil {
		applicability := fableResolver.Resolve(intent, defaultTarget)
		needsFable := applicability.needsFable()
		defaultUsageCtx, defaultUsageCancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
		defaultUsage, defaultUsageErr := fetchClaudeUsage(defaultUsageCtx, a.claudeCommandRunner(), "")
		defaultUsageCancel()
		if defaultUsageErr == nil && claudeUsageIsEligible(defaultUsage, needsFable) {
			eligible = append(eligible, claudeExecCandidate{
				Target: defaultTarget,
				Score:  claudeUsageWorstPercent(defaultUsage, needsFable),
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
		profileCopy := profile
		target := claudeTarget{Name: name, Kind: "managed", ConfigDir: profile.ConfigDir, Profile: &profileCopy}
		applicability := fableResolver.Resolve(intent, target)
		needsFable := applicability.needsFable()
		ctx, cancel := context.WithTimeout(context.Background(), claudeProbeTimeout)
		usage, usageErr := fetchClaudeUsage(ctx, a.claudeCommandRunner(), profile.ConfigDir)
		cancel()
		if usageErr != nil || !claudeUsageIsEligible(usage, needsFable) {
			continue
		}
		eligible = append(eligible, claudeExecCandidate{
			Target: target,
			Score:  claudeUsageWorstPercent(usage, needsFable),
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

func (a *App) claudeFableApplicabilityResolver() claudeFableApplicabilityResolver {
	if a.claudeFableResolver == nil {
		return newLocalClaudeFableApplicabilityResolver()
	}
	return a.claudeFableResolver
}

func claudeUsageIsEligible(usage claudeUsage, needsFable bool) bool {
	if usage.Session.UsedPercent >= 100 || usage.WeeklyAll.UsedPercent >= 100 {
		return false
	}
	if !needsFable {
		return true
	}
	return usage.Fable != nil && usage.Fable.UsedPercent < 100
}

func claudeUsageWorstPercent(usage claudeUsage, needsFable bool) float64 {
	worst := math.Max(usage.Session.UsedPercent, usage.WeeklyAll.UsedPercent)
	if needsFable && usage.Fable != nil {
		worst = math.Max(worst, usage.Fable.UsedPercent)
	}
	return worst
}
