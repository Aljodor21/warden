package main

import "net/http"

func (s *server) handleThemes(w http.ResponseWriter, r *http.Request) {
	render(w, "themes.html", map[string]any{"Page": "themes"})
}
