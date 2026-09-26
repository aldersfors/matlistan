package planner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/weekplan"
)

var errMissing = errors.New("missing")

type memStore struct {
	members  []household.Member
	plan     *weekplan.Plan
	cooked   []weekplan.Cooked
	library  map[int64]recipes.Recipe
	saved    []weekplan.Pick
	replaced bool
	errKey   string
	saveErr  error
	listErr  error
}

func (m *memStore) ListMembers(context.Context) ([]household.Member, error) {
	return m.members, m.listErr
}
func (m *memStore) GetSettings(context.Context) (household.Settings, error) {
	s := household.DefaultSettings()
	s.DinnersPerWeek = 2
	return s, nil
}
func (m *memStore) GetPlan(context.Context, weekplan.Key) (weekplan.Plan, error) {
	if m.plan == nil {
		return weekplan.Plan{}, errMissing
	}
	return *m.plan, nil
}
func (m *memStore) CookedSince(context.Context, weekplan.Key, weekplan.Key) ([]weekplan.Cooked, error) {
	return m.cooked, nil
}
func (m *memStore) ListCandidates(context.Context, i18n.Locale) ([]recipes.Recipe, error) {
	var out []recipes.Recipe
	for _, r := range m.library {
		out = append(out, r)
	}
	return out, nil
}
func (m *memStore) GetRecipe(_ context.Context, id int64) (recipes.Recipe, error) {
	r, ok := m.library[id]
	if !ok {
		return recipes.Recipe{}, errMissing
	}
	return r, nil
}
func (m *memStore) SavePicks(_ context.Context, _ weekplan.Key, _ weekplan.Context,
	p []weekplan.Pick, all bool) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved, m.replaced, m.errKey = p, all, ""
	return nil
}

// SetPlanError fails on an ended context, as a database call would.
func (m *memStore) SetPlanError(ctx context.Context, _ weekplan.Key, _ weekplan.Context, k string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.errKey = k
	return nil
}

type scripted struct {
	replies []string
	sent    []string
	err     error
}

func (s *scripted) NewSession(string, map[string]any) Session { return s }
func (s *scripted) Send(_ context.Context, text string) (Reply, error) {
	s.sent = append(s.sent, text)
	if s.err != nil {
		return Reply{}, s.err
	}
	return Reply{Text: s.replies[min(len(s.sent), len(s.replies))-1]}, nil
}

const (
	good = `{"days":[{"day":1,"why":"Säsong.","library_recipe_id":7,"new_recipe":null},` +
		`{"day":2,"why":"Snabbt.","library_recipe_id":null,"new_recipe":{"title":"Pumpasoppa",` +
		`"description":"","servings":2,"active_minutes":10,"total_minutes":30,"tags":[],` +
		`"steps":["Koka"],"diets":["vegetarian"],"allergens":[],"ingredients":[{"name":"pumpa",` +
		`"quantity":1,"unit":"kg","section":"produce","optional":false}]}}]}`
	repeat = `{"days":[{"day":1,"why":"x","library_recipe_id":7,"new_recipe":null},` +
		`{"day":2,"why":"x","library_recipe_id":7,"new_recipe":null}]}`
)

func newTestService(st *memStore, llm LLM) *Service {
	return NewService(st, llm, i18n.SV, time.UTC, zerolog.Nop(),
		func(err error) bool { return errors.Is(err, errMissing) })
}

func baseStore() *memStore {
	return &memStore{members: []household.Member{{ID: 1, Name: "Anna", BirthYear: 1985},
		{ID: 2, Name: "Erik", BirthYear: 1984}},
		library: map[int64]recipes.Recipe{7: {ID: 7, Title: "Ärtsoppa", TotalMinutes: 40,
			Lang: i18n.SV}}}
}

var _k = weekplan.Key{Year: 2026, Week: 40}

func TestGenerateFirstTry(t *testing.T) {
	st, llm := baseStore(), &scripted{replies: []string{good}}
	if err := newTestService(st, llm).Generate(context.Background(), _k); err != nil {
		t.Fatal(err)
	}
	if len(st.saved) != 2 || !st.replaced || st.saved[1].New.Title != "Pumpasoppa" ||
		len(llm.sent) != 1 || strings.Contains(llm.sent[0], "Anna") {
		t.Fatalf("saved %+v sent %d", st.saved, len(llm.sent))
	}
}

func TestGenerateRetriesWithFeedback(t *testing.T) {
	st, llm := baseStore(), &scripted{replies: []string{repeat, good}}
	if err := newTestService(st, llm).Generate(context.Background(), _k); err != nil {
		t.Fatal(err)
	}
	if len(llm.sent) != 2 || !strings.Contains(llm.sent[1], "already planned this week") {
		t.Fatalf("feedback = %q", llm.sent)
	}
}

// Review focus 1 and 2: two bad answers store the error and save nothing.
func TestGenerateGivesUpAfterTwoBadAnswers(t *testing.T) {
	for name, replies := range map[string][]string{"rules": {repeat, repeat},
		"not json": {"Sure! Here is a plan.", "{"}} {
		st := baseStore()
		err := newTestService(st, &scripted{replies: replies}).Generate(context.Background(), _k)
		if !errors.Is(err, ErrInvalid) || st.errKey != ErrorKeyInvalid || st.saved != nil {
			t.Errorf("%s: err %v, key %q, saved %v", name, err, st.errKey, st.saved)
		}
	}
}

func TestGenerateUnavailable(t *testing.T) {
	st := baseStore()
	err := newTestService(st, &scripted{err: errors.New("503")}).Generate(context.Background(), _k)
	if !errors.Is(err, ErrUnavailable) || st.errKey != ErrorKeyUnavailable {
		t.Fatalf("err %v key %q", err, st.errKey)
	}
}

func TestRecentDishesAreExcluded(t *testing.T) {
	st := baseStore()
	st.cooked = []weekplan.Cooked{{Key: _k.AddWeeks(-2), RecipeID: 7, Title: "Ärtsoppa"}}
	llm := &scripted{replies: []string{good, good}}
	_ = newTestService(st, llm).Generate(context.Background(), _k)
	if strings.Contains(llm.sent[0], `"id":7`) || !strings.Contains(llm.sent[0], `"recent":["Ärtsoppa"]`) {
		t.Fatalf("prompt = %s", llm.sent[0])
	}
}

func TestSwapReplacesOneDay(t *testing.T) {
	st := baseStore()
	st.plan = &weekplan.Plan{Key: _k, Status: weekplan.StatusDraft,
		Context: weekplan.DefaultContext(2),
		Entries: []weekplan.Entry{{Day: 1, RecipeID: 7, Title: "Ärtsoppa"},
			{Day: 2, RecipeID: 8, Title: "Tacos"}}}
	reply := `{"days":[{"day":2,"why":"Omväxling.","library_recipe_id":null,"new_recipe":` +
		`{"title":"Pumpasoppa","description":"","servings":2,"active_minutes":10,` +
		`"total_minutes":30,"tags":[],"steps":["Koka"],"diets":[],"allergens":[],` +
		`"ingredients":[{"name":"pumpa","quantity":1,"unit":"kg","section":"produce",` +
		`"optional":false}]}}]}`
	llm := &scripted{replies: []string{reply}}
	if err := newTestService(st, llm).Swap(context.Background(), _k, 2); err != nil {
		t.Fatal(err)
	}
	if st.replaced || len(st.saved) != 1 || st.saved[0].Day != 2 ||
		!strings.Contains(llm.sent[0], `"other_days_this_week":["Ärtsoppa"]`) {
		t.Fatalf("saved %+v prompt %s", st.saved, llm.sent[0])
	}
	if err := newTestService(st, llm).Swap(context.Background(), _k, 5); !errors.Is(err, ErrNotPlanned) {
		t.Fatalf("swap skipped day: %v", err)
	}
}

// Every failure leaves a message on the week, even when the job's own deadline has passed.
func TestFailuresAreAlwaysRecorded(t *testing.T) {
	cases := map[string]func(*memStore, *scripted) context.Context{
		"save fails": func(st *memStore, llm *scripted) context.Context {
			llm.replies = []string{good}
			st.saveErr = errors.New("db down")
			return context.Background()
		},
		"reading the week fails": func(st *memStore, _ *scripted) context.Context {
			st.listErr = errors.New("db down")
			return context.Background()
		},
		"deadline passed": func(_ *memStore, llm *scripted) context.Context {
			llm.err = context.DeadlineExceeded
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		},
	}
	for name, setup := range cases {
		st, llm := baseStore(), &scripted{}
		ctx := setup(st, llm)
		if err := newTestService(st, llm).Generate(ctx, _k); err == nil {
			t.Errorf("%s: no error", name)
		}
		if st.errKey != ErrorKeyUnavailable {
			t.Errorf("%s: error key %q, want %q", name, st.errKey, ErrorKeyUnavailable)
		}
	}
}
