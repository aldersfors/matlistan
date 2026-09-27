package web

import (
	"bytes"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/aldersfors/matlistan/internal/weekplan"

	"github.com/aldersfors/matlistan/internal/apitoken"
	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/shopping"
	"github.com/aldersfors/matlistan/internal/store"
)

// fakeStore is an in-memory Store with the same not-found and ordering rules as Postgres.
type fakeStore struct {
	mu          sync.Mutex
	pingErr     error
	nextID      int64
	members     map[int64]household.Member
	settings    household.Settings
	staples     map[int64]household.Staple
	recipes     map[int64]recipes.Recipe
	plans       map[weekplan.Key]weekplan.Plan
	lists       map[weekplan.Key]shopping.List
	ingredients map[weekplan.Key][]shopping.Use
	tokens      map[int64]fakeToken
	ratings     map[weekplan.Key]map[int]map[int64]int
}

func newFakeStore() *fakeStore {
	return &fakeStore{members: map[int64]household.Member{}, staples: map[int64]household.Staple{},
		recipes: map[int64]recipes.Recipe{}, settings: household.DefaultSettings(),
		plans: map[weekplan.Key]weekplan.Plan{}, lists: map[weekplan.Key]shopping.List{},
		ingredients: map[weekplan.Key][]shopping.Use{}, tokens: map[int64]fakeToken{},
		ratings: map[weekplan.Key]map[int]map[int64]int{}}
}

func (f *fakeStore) id() int64 { f.nextID++; return f.nextID }

func (f *fakeStore) Ping(context.Context) error { return f.pingErr }

func (f *fakeStore) ListMembers(context.Context) ([]household.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Collect(maps.Values(f.members))
	slices.SortFunc(out, func(a, b household.Member) int {
		if a.BirthYear != b.BirthYear {
			return a.BirthYear - b.BirthYear
		}
		return int(a.ID - b.ID)
	})
	return out, nil
}

func (f *fakeStore) GetMember(_ context.Context, id int64) (household.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.members[id]
	if !ok {
		return household.Member{}, store.ErrNotFound
	}
	return m, nil
}

func (f *fakeStore) CreateMember(_ context.Context, m household.Member) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m.ID, m.Subject = f.id(), ""
	f.members[m.ID] = m
	return m.ID, nil
}

func (f *fakeStore) UpdateMember(_ context.Context, m household.Member) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	old, ok := f.members[m.ID]
	if !ok {
		return store.ErrNotFound
	}
	m.Subject = old.Subject
	f.members[m.ID] = m
	return nil
}

func (f *fakeStore) ArchiveMember(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.members[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.members, id)
	return nil
}

func (f *fakeStore) LinkMember(_ context.Context, id int64, subject string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.members[id]; !ok {
		return store.ErrNotFound
	}
	for k, m := range f.members {
		if m.Subject == subject {
			m.Subject = ""
			f.members[k] = m
		}
	}
	m := f.members[id]
	m.Subject = subject
	f.members[id] = m
	return nil
}

func (f *fakeStore) GetSettings(context.Context) (household.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings, nil
}

func (f *fakeStore) UpdateSettings(_ context.Context, s household.Settings) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings = s
	return nil
}

func (f *fakeStore) ListStaples(context.Context) ([]household.Staple, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Collect(maps.Values(f.staples))
	slices.SortFunc(out, func(a, b household.Staple) int {
		return strings.Compare(household.StapleKey(a.Name), household.StapleKey(b.Name))
	})
	return out, nil
}

func (f *fakeStore) AddStaple(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.staples {
		if household.StapleKey(s.Name) == household.StapleKey(name) {
			return nil
		}
	}
	id := f.id()
	f.staples[id] = household.Staple{ID: id, Name: household.StapleName(name)}
	return nil
}

func (f *fakeStore) RemoveStaple(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.staples[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.staples, id)
	return nil
}

func (f *fakeStore) ListRecipes(_ context.Context, lang i18n.Locale, q string) (
	[]recipes.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recipes.Summary
	for _, r := range f.recipes {
		if r.Lang == lang && r.Source != "generated" &&
			strings.Contains(recipes.TitleKey(r.Title), recipes.TitleKey(q)) {
			out = append(out, recipes.Summary{ID: r.ID, Title: r.Title,
				TotalMinutes: r.TotalMinutes, Tags: r.Tags, Rating: r.Rating})
		}
	}
	slices.SortFunc(out, func(a, b recipes.Summary) int {
		return strings.Compare(recipes.TitleKey(a.Title), recipes.TitleKey(b.Title))
	})
	return out, nil
}

func (f *fakeStore) GetRecipe(_ context.Context, id int64) (recipes.Recipe, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.recipes[id]
	if !ok {
		return recipes.Recipe{}, store.ErrNotFound
	}
	return r, nil
}

func (f *fakeStore) CreateRecipe(_ context.Context, r recipes.Recipe) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, other := range f.recipes {
		if r.SourceURL != "" && other.SourceURL == r.SourceURL {
			return 0, store.ErrDuplicateSource
		}
	}
	r.ID = f.id()
	f.recipes[r.ID] = r
	return r.ID, nil
}

func (f *fakeStore) UpdateRecipe(_ context.Context, r recipes.Recipe) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.recipes[r.ID]; !ok {
		return store.ErrNotFound
	}
	f.recipes[r.ID] = r
	return nil
}

func (f *fakeStore) ArchiveRecipe(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.recipes[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.recipes, id)
	return nil
}

func (f *fakeStore) GetPlan(_ context.Context, k weekplan.Key) (weekplan.Plan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.plans[k]
	if !ok {
		return weekplan.Plan{}, store.ErrNotFound
	}
	return p, nil
}

func (f *fakeStore) SaveContext(_ context.Context, k weekplan.Key, c weekplan.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.plans[k]
	if ok && p.Status == weekplan.StatusApproved {
		return store.ErrApproved
	}
	p.Key, p.Context = k, c
	if p.Status == "" {
		p.Status = weekplan.StatusDraft
	}
	kept := p.Entries[:0:0]
	for _, e := range p.Entries {
		if !c.Days[e.Day-1].Skip {
			kept = append(kept, e)
		}
	}
	p.Entries = kept
	f.plans[k] = p
	return nil
}

func (f *fakeStore) ApprovePlan(_ context.Context, k weekplan.Key, subject string,
	items []shopping.Item, excluded int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.plans[k]
	if !ok || p.Status != weekplan.StatusDraft || len(p.Entries) == 0 {
		return store.ErrNotFound
	}
	p.Status, p.ApprovedBy = weekplan.StatusApproved, subject
	f.plans[k] = p
	l := shopping.List{ID: f.id(), Key: k, Excluded: excluded}
	for _, it := range items {
		it.ID = f.id()
		l.Items = append(l.Items, it)
	}
	f.lists[k] = l
	return nil
}

func (f *fakeStore) PlanIngredients(_ context.Context, k weekplan.Key) ([]shopping.Use, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ingredients[k], nil
}

func (f *fakeStore) GetShoppingList(_ context.Context, k weekplan.Key) (shopping.List, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.lists[k]
	if !ok {
		return shopping.List{}, store.ErrNotFound
	}
	return l, nil
}

func (f *fakeStore) CurrentShoppingList(_ context.Context, upTo weekplan.Key) (shopping.List,
	error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *shopping.List
	for k, l := range f.lists {
		if upTo.Less(k) {
			continue
		}
		if best == nil || best.Key.Less(k) {
			l := l
			best = &l
		}
	}
	if best == nil {
		return shopping.List{}, store.ErrNotFound
	}
	return *best, nil
}

// item finds an item by id; the caller holds f.mu.
func (f *fakeStore) item(id int64) (weekplan.Key, int, bool) {
	for k, l := range f.lists {
		for i, it := range l.Items {
			if it.ID == id {
				return k, i, true
			}
		}
	}
	return weekplan.Key{}, 0, false
}

func (f *fakeStore) SetItemChecked(_ context.Context, id int64, checked bool) (shopping.Item,
	error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, i, ok := f.item(id)
	if !ok {
		return shopping.Item{}, store.ErrNotFound
	}
	l := f.lists[k]
	l.Items[i].Checked = checked
	return l.Items[i], nil
}

func (f *fakeStore) AddManualItem(_ context.Context, listID int64, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, l := range f.lists {
		if l.ID != listID {
			continue
		}
		manual := 0
		for _, it := range l.Items {
			if it.Manual {
				manual++
			}
		}
		if manual >= 100 {
			return store.ErrTooMany
		}
		l.Items = append(l.Items, shopping.Item{ID: f.id(), Name: name, Section: "other",
			Manual: true})
		f.lists[k] = l
		return nil
	}
	return store.ErrNotFound
}

func (f *fakeStore) RemoveManualItem(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, i, ok := f.item(id)
	if !ok || !f.lists[k].Items[i].Manual {
		return store.ErrNotFound
	}
	l := f.lists[k]
	l.Items = append(l.Items[:i], l.Items[i+1:]...)
	f.lists[k] = l
	return nil
}

type fakePlanner struct {
	st      *fakeStore
	err     error
	gate    chan struct{} // when set, Generate waits for it (to observe "planning")
	swapped []int
}

func (p *fakePlanner) Generate(_ context.Context, k weekplan.Key) error {
	if p.gate != nil {
		<-p.gate
	}
	if p.err != nil {
		p.st.mu.Lock()
		pl := p.st.plans[k]
		pl.Key, pl.Status, pl.Error = k, weekplan.StatusDraft, "plan.error.invalid"
		p.st.plans[k] = pl
		p.st.mu.Unlock()
		return p.err
	}
	p.st.mu.Lock()
	defer p.st.mu.Unlock()
	pl := p.st.plans[k]
	pl.Key, pl.Status, pl.Error = k, weekplan.StatusDraft, ""
	if pl.Context.Days[0].Away == nil {
		pl.Context = weekplan.DefaultContext(7)
	}
	pl.Entries = []weekplan.Entry{{Day: 1, RecipeID: 1, Title: "Pumpasoppa", TotalMinutes: 30,
		Servings: 4, Why: "Pumpan är i säsong."}, {Day: 2, RecipeID: 2, Title: "Köttbullar",
		TotalMinutes: 40, Servings: 4, Why: "Alla fyra gav den fem av fem."}}
	p.st.plans[k] = pl
	return nil
}

func (p *fakePlanner) Swap(_ context.Context, _ weekplan.Key, day int) error {
	p.swapped = append(p.swapped, day)
	return nil
}

// newPlanningServer is newServer with a planner; the returned *server lets a test wait for
// background jobs.
func newPlanningServer(t *testing.T, l i18n.Locale, st *fakeStore, pl Planner) (http.Handler,
	*server) {
	t.Helper()
	c, err := i18n.Load(l)
	if err != nil {
		t.Fatal(err)
	}
	sthlm, _ := time.LoadLocation("Europe/Stockholm")
	s := buildServer(Deps{Catalog: c, Auth: fakeAuth{signedIn: true}, Store: st, Planner: pl,
		Now:     func() time.Time { return time.Date(2026, 9, 27, 8, 0, 0, 0, sthlm) },
		BaseURL: "https://matlistan.example.lan", Log: zerolog.Nop()})
	return s.handler(), s
}

type fakeToken struct {
	apitoken.Token
	hash []byte
}

func (f *fakeStore) CreateAPIToken(_ context.Context, _, name string, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.id()
	f.tokens[id] = fakeToken{Token: apitoken.Token{ID: id, Name: name, CreatedAt: time.Now()},
		hash: hash}
	return nil
}

func (f *fakeStore) ListAPITokens(context.Context) ([]apitoken.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []apitoken.Token
	for _, t := range f.tokens {
		out = append(out, t.Token)
	}
	slices.SortFunc(out, func(a, b apitoken.Token) int { return int(b.ID - a.ID) })
	return out, nil
}

func (f *fakeStore) RevokeAPIToken(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tokens[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.tokens, id)
	return nil
}

func (f *fakeStore) UseAPIToken(_ context.Context, hash []byte) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, t := range f.tokens {
		if bytes.Equal(t.hash, hash) {
			now := time.Now()
			t.LastUsedAt = &now
			f.tokens[id] = t
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) SetRating(_ context.Context, k weekplan.Key, day int, memberID int64,
	score int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Contains(recipes.Scores, score) {
		return errors.New("score must be 1, 3 or 5")
	}
	p, ok := f.plans[k]
	if _, isEntry := p.Entry(day); !ok || p.Status != weekplan.StatusApproved || !isEntry {
		return store.ErrNotFound
	}
	if _, ok := f.members[memberID]; !ok {
		return store.ErrNotFound
	}
	if f.ratings[k] == nil {
		f.ratings[k] = map[int]map[int64]int{}
	}
	if f.ratings[k][day] == nil {
		f.ratings[k][day] = map[int64]int{}
	}
	f.ratings[k][day][memberID] = score
	return nil
}

func (f *fakeStore) WeekRatings(_ context.Context, k weekplan.Key) (map[int]map[int64]int,
	error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int]map[int64]int{}
	for d, m := range f.ratings[k] {
		out[d] = maps.Clone(m)
	}
	return out, nil
}

func (f *fakeStore) FindRecipeBySourceURL(_ context.Context, u string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, r := range f.recipes {
		if r.SourceURL == u {
			return id, nil
		}
	}
	return 0, store.ErrNotFound
}
