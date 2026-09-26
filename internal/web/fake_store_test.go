package web

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/store"
)

// fakeStore is an in-memory Store with the same not-found and ordering rules as Postgres.
type fakeStore struct {
	mu       sync.Mutex
	pingErr  error
	nextID   int64
	members  map[int64]household.Member
	settings household.Settings
	staples  map[int64]household.Staple
	recipes  map[int64]recipes.Recipe
}

func newFakeStore() *fakeStore {
	return &fakeStore{members: map[int64]household.Member{}, staples: map[int64]household.Staple{},
		recipes: map[int64]recipes.Recipe{}, settings: household.DefaultSettings()}
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
		if r.Lang == lang && strings.Contains(recipes.TitleKey(r.Title), recipes.TitleKey(q)) {
			out = append(out, recipes.Summary{ID: r.ID, Title: r.Title,
				TotalMinutes: r.TotalMinutes, Tags: r.Tags})
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
