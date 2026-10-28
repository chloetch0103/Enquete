package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Enquete/sma"
)

func TestAPI(t *testing.T) {
	p := sma.NouveauPilote(func() *sma.Monde { return sma.NouveauMonde(sma.ConfigParDefaut(), nil) }, time.Millisecond)
	srv := httptest.NewServer(Routeur(p))
	defer srv.Close()

	r, err := http.Get(srv.URL + "/api/etat")
	if err != nil {
		t.Fatal(err)
	}
	var in sma.Instantane
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Agents) != 30 || !in.EnPause {
		t.Fatalf("état initial inattendu : %d agents, pause=%v, err=%v", len(in.Agents), in.EnPause, err)
	}
	r.Body.Close()

	for corps, attendu := range map[string]int{
		`{"action":"vitesse","delai_ms":10}`:   http.StatusNoContent,
		`{"action":"vitesse","delai_ms":9999}`: http.StatusBadRequest,
		`{"action":"inconnue"}`:                http.StatusBadRequest,
		`pas du json`:                          http.StatusBadRequest,
	} {
		r, err := http.Post(srv.URL+"/api/commande", "application/json", strings.NewReader(corps))
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != attendu {
			t.Errorf("%s → %d, attendu %d", corps, r.StatusCode, attendu)
		}
	}

	r, err = http.Get(srv.URL + "/")
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("page d'accueil : %v %v", r.StatusCode, err)
	}
	r.Body.Close()
}
