package store

import "context"

// SettingOnboardingDismissed hides the setup checklist even if incomplete.
const SettingOnboardingDismissed = "onboarding_dismissed"

// onboardingSkipKey is the per-step "skip for now" setting key.
func onboardingSkipKey(step string) string { return "onboarding_skip_" + step }

// OnboardingStep is one item in the first-run setup checklist.
type OnboardingStep struct {
	Key     string
	Label   string
	Help    string
	Link    string
	Done    bool
	Skipped bool // deferred by the admin ("skip for now"); still reachable
}

// Onboarding is the computed setup progress for the dashboard checklist.
type Onboarding struct {
	Steps        []OnboardingStep
	DoneCount    int
	SkippedCount int
	PendingCount int
	Total        int
	Percent      int
	AllDone      bool // every step done → hide the checklist entirely
}

// OnboardingStatus computes which setup steps are done (from data presence) or
// skipped (deferred by the admin) so the dashboard shows a guided checklist.
func (s *Store) OnboardingStatus(ctx context.Context) (*Onboarding, error) {
	cfg, err := s.AllConfig(ctx)
	if err != nil {
		return nil, err
	}
	officers, _ := s.ListRoster(ctx, RosterOfficer)
	pages, _ := s.ListPages(ctx)
	events, _ := s.ListEvents(ctx)
	members, _ := s.ListMembers(ctx)

	infoDone := cfg["postName"] != "" && cfg["email"] != "" &&
		(cfg["meetingSchedule"] != "" || cfg["meetingLocation"] != "")

	steps := []OnboardingStep{
		{Key: "info", Label: "Add your post's info", Help: "Name, contact, and meeting times.", Link: "/content/info", Done: infoDone},
		{Key: "roster", Label: "Add your officers", Help: "Your officer and Legion-family roster.", Link: "/content/roster", Done: len(officers) > 0},
		{Key: "pages", Label: "Write your pages", Help: "History, hall rental, membership, and more.", Link: "/content/pages", Done: len(pages) > 0},
		{Key: "events", Label: "Add an event", Help: "Meetings and community events for your site.", Link: "/events/new", Done: len(events) > 0},
		{Key: "members", Label: "Add members for reminders", Help: "Optional — powers SMS event reminders.", Link: "/members/new", Done: len(members) > 0},
	}

	ob := &Onboarding{Total: len(steps)}
	for i := range steps {
		if !steps[i].Done {
			// A completed step is never "skipped"; skip only applies to pending ones.
			steps[i].Skipped, _ = s.GetSettingBool(ctx, onboardingSkipKey(steps[i].Key), false)
		}
		switch {
		case steps[i].Done:
			ob.DoneCount++
		case steps[i].Skipped:
			ob.SkippedCount++
		default:
			ob.PendingCount++
		}
	}
	ob.Steps = steps
	ob.Percent = ob.DoneCount * 100 / ob.Total
	ob.AllDone = ob.DoneCount == ob.Total
	return ob, nil
}

// SetOnboardingSkip marks a step as skipped (true) or clears it (false).
func (s *Store) SetOnboardingSkip(ctx context.Context, step string, skip bool) error {
	return s.SetSettingBool(ctx, onboardingSkipKey(step), skip)
}
