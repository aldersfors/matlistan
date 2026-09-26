package planner

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/rs/zerolog"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/weekplan"
)

// Error keys stored on a week, shown by the web page.
const (
	ErrorKeyInvalid     = "plan.error.invalid"
	ErrorKeyUnavailable = "plan.error.unavailable"
)

// Service errors.
var (
	ErrInvalid     = errors.New("no valid plan after one retry")
	ErrUnavailable = errors.New("the model could not be reached")
	ErrNotPlanned  = errors.New("that day has no dinner planned")
)

const (
	_historyWeeks  = 26
	_candidatesMax = 60
)

// Store is what planning reads and writes.
type Store interface {
	ListMembers(ctx context.Context) ([]household.Member, error)
	GetSettings(ctx context.Context) (household.Settings, error)
	GetPlan(ctx context.Context, k weekplan.Key) (weekplan.Plan, error)
	CookedSince(ctx context.Context, from, until weekplan.Key) ([]weekplan.Cooked, error)
	ListCandidates(ctx context.Context, lang i18n.Locale) ([]recipes.Recipe, error)
	GetRecipe(ctx context.Context, id int64) (recipes.Recipe, error)
	SavePicks(ctx context.Context, k weekplan.Key, c weekplan.Context, p []weekplan.Pick,
		replaceAll bool) error
	SetPlanError(ctx context.Context, k weekplan.Key, c weekplan.Context, key string) error
}

// Service plans weeks and swaps dinners.
type Service struct {
	st       Store
	llm      LLM
	locale   i18n.Locale
	loc      *time.Location
	log      zerolog.Logger
	notFound func(error) bool
}

// NewService wires planning; notFound recognises the store's "no such row" error.
func NewService(st Store, llm LLM, l i18n.Locale, loc *time.Location, log zerolog.Logger,
	notFound func(error) bool) *Service {
	return &Service{st: st, llm: llm, locale: l, loc: loc, log: log, notFound: notFound}
}

// Generate plans every planned day of week k, replacing its dinners.
func (s *Service) Generate(ctx context.Context, k weekplan.Key) error {
	req, plan, err := s.request(ctx, k)
	if err != nil {
		return err
	}
	return s.run(ctx, "week", req, plan, true)
}

// Swap plans day again, keeping the other days.
func (s *Service) Swap(ctx context.Context, k weekplan.Key, day int) error {
	req, plan, err := s.request(ctx, k)
	if err != nil {
		return err
	}
	if day < 1 || day > 7 || !req.Days[day-1].Planned {
		return ErrNotPlanned
	}
	req.SwapDay = day
	for _, e := range plan.Entries {
		if e.Day != day {
			req.Keep = append(req.Keep, e.Title)
		}
	}
	return s.run(ctx, "swap", req, plan, false)
}

func (s *Service) run(ctx context.Context, kind string, req Request, plan weekplan.Plan,
	replaceAll bool) error {
	start := time.Now()
	defer func() { _duration.Observe(time.Since(start).Seconds()) }()
	picks, err := s.propose(ctx, req)
	switch {
	case errors.Is(err, ErrInvalid):
		_runs.WithLabelValues(kind, "invalid").Inc()
		return errors.Join(err, s.st.SetPlanError(ctx, req.Key, plan.Context, ErrorKeyInvalid))
	case err != nil:
		_runs.WithLabelValues(kind, "unavailable").Inc()
		s.log.Warn().Err(err).Str("week", req.Key.String()).Msg("planner unavailable")
		return errors.Join(ErrUnavailable,
			s.st.SetPlanError(ctx, req.Key, plan.Context, ErrorKeyUnavailable))
	}
	if err := s.st.SavePicks(ctx, req.Key, plan.Context, picks, replaceAll); err != nil {
		return fmt.Errorf("save plan: %w", err)
	}
	_runs.WithLabelValues(kind, "ok").Inc()
	return nil
}

// propose asks once, and once more with feedback if the first answer breaks a rule.
func (s *Service) propose(ctx context.Context, req Request) ([]weekplan.Pick, error) {
	prompt, err := UserPrompt(req)
	if err != nil {
		return nil, err
	}
	sess := s.llm.NewSession(SystemPrompt(s.locale), Schema())
	text := prompt
	for attempt := 1; attempt <= 2; attempt++ {
		reply, err := sess.Send(ctx, text)
		countUsage(reply.Usage)
		if errors.Is(err, ErrTruncated) {
			text = "Your answer was cut off. Answer again with a shorter complete JSON object."
			continue
		}
		if err != nil {
			return nil, err
		}
		p, err := ParseProposal(reply.Text)
		if err != nil {
			text = "Your answer was not the requested JSON object (" + err.Error() +
				"). Answer again with only the JSON object."
			continue
		}
		library, err := s.libraryFor(ctx, p)
		if err != nil {
			return nil, err
		}
		picks, vs := Resolve(req, p, library)
		if len(vs) == 0 {
			return picks, nil
		}
		s.log.Info().Int("violations", len(vs)).Int("attempt", attempt).
			Str("week", req.Key.String()).Msg("plan rejected")
		text = Feedback(vs)
	}
	return nil, ErrInvalid
}

// libraryFor loads the full recipes a proposal names; unknown ids are left out so Resolve
// reports them.
func (s *Service) libraryFor(ctx context.Context, p Proposal) (map[int64]recipes.Recipe, error) {
	out := map[int64]recipes.Recipe{}
	for _, d := range p.Days {
		if d.LibraryRecipeID == nil {
			continue
		}
		r, err := s.st.GetRecipe(ctx, *d.LibraryRecipeID)
		switch {
		case s.notFound(err):
			continue
		case err != nil:
			return nil, err
		}
		out[r.ID] = r
	}
	return out, nil
}

// request gathers the week: members, rules, conditions, history and candidates.
func (s *Service) request(ctx context.Context, k weekplan.Key) (Request, weekplan.Plan, error) {
	members, err := s.st.ListMembers(ctx)
	if err != nil {
		return Request{}, weekplan.Plan{}, err
	}
	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return Request{}, weekplan.Plan{}, err
	}
	plan, err := s.st.GetPlan(ctx, k)
	switch {
	case s.notFound(err):
		plan = weekplan.Plan{Key: k, Status: weekplan.StatusDraft,
			Context: weekplan.DefaultContext(settings.DinnersPerWeek)}
	case err != nil:
		return Request{}, weekplan.Plan{}, err
	}
	cooked, err := s.st.CookedSince(ctx, k.AddWeeks(-_historyWeeks), k)
	if err != nil {
		return Request{}, weekplan.Plan{}, err
	}
	library, err := s.st.ListCandidates(ctx, s.locale)
	if err != nil {
		return Request{}, weekplan.Plan{}, err
	}
	monday := k.Monday(s.loc)
	req := Request{Key: k, Locale: s.locale, Month: monday.Month(), Settings: settings,
		Days: weekplan.Specs(k, s.loc, plan.Context, members, settings)}
	for _, m := range members {
		req.Members = append(req.Members, Member{Age: household.Age(m.BirthYear, monday),
			Diets: m.Diets, Allergens: m.Allergens, Likes: m.Likes, Dislikes: m.Dislikes})
		req.Allergens = append(req.Allergens, m.Allergens...)
		req.Diets = append(req.Diets, m.Diets...)
	}
	slices.Sort(req.Allergens)
	req.Allergens = slices.Compact(req.Allergens)
	slices.Sort(req.Diets)
	req.Diets = slices.Compact(req.Diets)

	windowStart := k.AddWeeks(-settings.RepeatWindowWeeks)
	lastCooked := map[int64]weekplan.Key{}
	recent := map[int64]bool{}
	for _, c := range cooked {
		lastCooked[c.RecipeID] = c.Key
		if !c.Key.Less(windowStart) {
			recent[c.RecipeID] = true
			if !slices.Contains(req.Recent, c.Title) {
				req.Recent = append(req.Recent, c.Title)
			}
		}
	}
	for _, r := range library {
		if recent[r.ID] {
			continue
		}
		c := Candidate{ID: r.ID, Title: r.Title, TotalMinutes: r.TotalMinutes, Tags: r.Tags,
			Diets: r.Diets, Allergens: r.Allergens}
		if last, ok := lastCooked[r.ID]; ok {
			c.WeeksSinceCooked = weeksBetween(last, k)
		}
		req.Candidates = append(req.Candidates, c)
	}
	// Longest since cooked first; never cooked counts as longest.
	slices.SortStableFunc(req.Candidates, func(a, b Candidate) int {
		return rank(b) - rank(a)
	})
	req.Candidates = req.Candidates[:min(len(req.Candidates), _candidatesMax)]
	return req, plan, nil
}

func rank(c Candidate) int {
	if c.WeeksSinceCooked == 0 {
		return _historyWeeks + 1
	}
	return c.WeeksSinceCooked
}

func weeksBetween(a, b weekplan.Key) int {
	return int(b.Monday(time.UTC).Sub(a.Monday(time.UTC)).Hours() / (24 * 7))
}
