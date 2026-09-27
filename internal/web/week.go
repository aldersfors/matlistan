package web

import (
	"net/http"
	"strconv"

	"github.com/jalet/matlistan/internal/web/views"
	"github.com/jalet/matlistan/internal/week"
)

func (s *server) week(w http.ResponseWriter, r *http.Request) {
	c := s.Catalog
	wk := week.Upcoming(s.Now())
	vm := views.Week{
		Label: c.WeekLabel(wk.Number),
		Range: c.T("week.range", "from", c.Date(wk.Days[0]), "to", c.Date(wk.Days[6])),
	}
	for i, d := range wk.Days {
		vm.Days = append(vm.Days, views.Day{Index: i + 1, Name: c.WeekdayShort(d.Weekday()),
			Date: strconv.Itoa(d.Day())})
	}
	if err := views.WeekPage(vm).Render(r.Context(), w); err != nil {
		s.Log.Error().Err(err).Msg("render week")
	}
}
