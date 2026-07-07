package store

import "context"

// SettingOnboardingDismissed hides the setup checklist even if incomplete.
const SettingOnboardingDismissed = "onboarding_dismissed"

// OnboardingStep is one item in the first-run setup checklist.
type OnboardingStep struct {
	Key   string
	Label string
	Help  string
	Link  string
	Done  bool
}

// Onboarding is the computed setup progress for the dashboard checklist.
type Onboarding struct {
	Steps    []OnboardingStep
	DoneCount int
	Total     int
	Percent   int
	Complete  bool
}

// OnboardingStatus computes which setup steps are done from data presence, so
// a new post's admin sees a guided checklist that fills in as they work.
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
		{"info", "Add your post's info", "Name, contact, and meeting times.", "/content/info", infoDone},
		{"roster", "Add your officers", "Your officer and Legion-family roster.", "/content/roster", len(officers) > 0},
		{"pages", "Write your pages", "History, hall rental, membership, and more.", "/content/pages", len(pages) > 0},
		{"events", "Add an event", "Meetings and community events for your site.", "/events/new", len(events) > 0},
		{"members", "Add members for reminders", "Optional — powers SMS event reminders.", "/members/new", len(members) > 0},
	}

	ob := &Onboarding{Steps: steps, Total: len(steps)}
	for _, st := range steps {
		if st.Done {
			ob.DoneCount++
		}
	}
	ob.Percent = ob.DoneCount * 100 / ob.Total
	ob.Complete = ob.DoneCount == ob.Total
	return ob, nil
}
