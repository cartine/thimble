package web

import (
	"errors"
	"net/http"
)

func (s *Server) handleStore(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectErr(w, r, err)
		return
	}
	name := r.FormValue("name")
	var err error
	switch r.FormValue("action") {
	case "create":
		_, err = s.stores.Create(name)
	case "select":
		_, err = s.stores.Select(name)
	default:
		err = errors.New("unknown store action")
	}
	if err != nil {
		s.redirectErr(w, r, err)
		return
	}
	s.redirectNotice(w, r, "active store: "+name)
}
