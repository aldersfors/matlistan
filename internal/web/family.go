package web

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/aldersfors/matlistan/internal/auth"
	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/validate"
	"github.com/aldersfors/matlistan/internal/web/views"
)

func (s *server) family(w http.ResponseWriter, r *http.Request) {
	members, err := s.Store.ListMembers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	me, _ := auth.SessionFrom(r.Context())
	var page views.Family
	for _, m := range members {
		row := views.MemberRow{ID: m.ID, Name: m.Name, Age: household.Age(m.BirthYear, s.Now()),
			IsMe: me.Subject != "" && m.Subject == me.Subject}
		for _, d := range m.Diets {
			row.Chips = append(row.Chips, s.Catalog.Diet(d))
		}
		for _, a := range m.Allergens {
			row.Chips = append(row.Chips, s.Catalog.Allergen(a))
		}
		page.Linked = page.Linked || row.IsMe
		page.Members = append(page.Members, row)
	}
	s.render(w, r, http.StatusOK, views.FamilyPage(page))
}

func (s *server) newMember(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, views.MemberFormPage(s.memberForm(household.Member{}, "", nil)))
}

func (s *server) editMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	m, err := s.Store.GetMember(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := s.memberForm(m, strconv.Itoa(m.BirthYear), nil)
	s.render(w, r, http.StatusOK, views.MemberFormPage(f))
}

func (s *server) createMember(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	m, year, e := s.parseMember(r)
	if len(e) > 0 {
		s.render(w, r, http.StatusUnprocessableEntity, views.MemberFormPage(s.memberForm(m, year, e)))
		return
	}
	if _, err := s.Store.CreateMember(r.Context(), m); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/family", http.StatusSeeOther)
}

func (s *server) updateMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if !readForm(w, r) {
		return
	}
	m, year, e := s.parseMember(r)
	m.ID = id
	if len(e) > 0 {
		s.render(w, r, http.StatusUnprocessableEntity, views.MemberFormPage(s.memberForm(m, year, e)))
		return
	}
	if err := s.Store.UpdateMember(r.Context(), m); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/family", http.StatusSeeOther)
}

func (s *server) archiveMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := s.Store.ArchiveMember(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/family", http.StatusSeeOther)
}

func (s *server) linkMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	me, signedIn := auth.SessionFrom(r.Context())
	if !ok || !signedIn || me.Subject == "" {
		s.notFound(w, r)
		return
	}
	if err := s.Store.LinkMember(r.Context(), id, me.Subject); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/family", http.StatusSeeOther)
}

// parseMember reads the member form. year is the birth year as typed, for re-display.
func (s *server) parseMember(r *http.Request) (household.Member, string, validate.Errors) {
	year := strings.TrimSpace(r.PostFormValue("birth_year"))
	e := validate.Errors{}
	y, err := strconv.Atoi(year)
	if err != nil {
		e.Add("birth_year", "form.not_a_number")
	}
	m := household.Member{Name: r.PostFormValue("name"), BirthYear: y,
		Diets: r.PostForm["diets"], Allergens: r.PostForm["allergens"],
		Likes: r.PostFormValue("likes"), Dislikes: r.PostFormValue("dislikes")}.Clean()
	e.Merge(m.Validate(s.Now()))
	return m, year, e
}

func (s *server) memberForm(m household.Member, year string, e validate.Errors) views.MemberForm {
	return views.MemberForm{ID: m.ID, Name: m.Name, BirthYear: year, Likes: m.Likes,
		Dislikes: m.Dislikes, Errors: e,
		Diets:     options(household.Diets, m.Diets, s.Catalog.Diet),
		Allergens: options(household.Allergens, m.Allergens, s.Catalog.Allergen)}
}

// options builds checkbox choices in the order of all, labelled and ticked.
func options(all, checked []string, label func(string) string) []views.Option {
	out := make([]views.Option, len(all))
	for i, v := range all {
		out[i] = views.Option{Value: v, Label: label(v), Checked: slices.Contains(checked, v)}
	}
	return out
}
