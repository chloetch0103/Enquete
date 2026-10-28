package sma

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fauxOllama simule l'API /api/chat d'Ollama avec une réponse fixe
func fauxOllama(t *testing.T, contenu string, statut int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("chemin inattendu %s", r.URL.Path)
		}
		var req requeteChat
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Format != "json" || len(req.Messages) != 2 {
			t.Errorf("requête mal formée : %+v (%v)", req, err)
		}
		w.WriteHeader(statut)
		_ = json.NewEncoder(w).Encode(reponseChat{Message: messageChat{Role: "assistant", Content: contenu}})
	}))
}

func strategieTest(url string, t TypeAgent, journal *JournalLLM) Strategie {
	return FabriqueLLM(NouveauClientLLM(url, "test", 2*time.Second, 2), journal)(t)
}

func TestLLMDecisionValide(t *testing.T) {
	srv := fauxOllama(t, `{"decision": "refuse", "raison": "trop gourmand"}`, http.StatusOK)
	defer srv.Close()
	var buf bytes.Buffer
	j := NouveauJournalLLM(&buf)
	// Un altruiste "à règles" accepterait toujours : ici c'est le LLM qui décide
	d := strategieTest(srv.URL, Altruiste, j).Repondre(ContexteNegociation{DemandeProposant: 0.7})
	if d.Accepte || !strings.Contains(d.Raison, "[llm]") || !strings.Contains(d.Raison, "trop gourmand") {
		t.Fatalf("décision inattendue : %+v", d)
	}
	if !strings.Contains(buf.String(), `"source":"llm"`) {
		t.Fatalf("journal JSONL incomplet : %s", buf.String())
	}
}

func TestLLMRepliSurReponseInvalide(t *testing.T) {
	srv := fauxOllama(t, `je ne sais pas`, http.StatusOK)
	defer srv.Close()
	j := NouveauJournalLLM(nil)
	d := strategieTest(srv.URL, Altruiste, j).Repondre(ContexteNegociation{DemandeProposant: 0.7})
	if !d.Accepte || !strings.Contains(d.Raison, "[repli]") {
		t.Fatalf("le repli sur les règles n'a pas eu lieu : %+v", d)
	}
	if !strings.Contains(j.Resume(), "1 replis") {
		t.Fatalf("résumé : %s", j.Resume())
	}
}

func TestLLMRepliSiServeurIndisponible(t *testing.T) {
	srv := fauxOllama(t, "", http.StatusInternalServerError)
	defer srv.Close()
	d := strategieTest(srv.URL, Neutre, nil).Repondre(ContexteNegociation{DemandeProposant: 0.7})
	if d.Accepte || !strings.Contains(d.Raison, "[repli]") {
		t.Fatalf("attendu un refus par repli (neutre, 70 %%) : %+v", d)
	}
}

func TestAnalyserDecision(t *testing.T) {
	cas := map[string]bool{
		`{"decision":"accepte","raison":"ok"}`:                          true,
		"```json\n{\"decision\": \"Refuse\", \"raison\": \"non\"}\n```": false,
		`Voici : {"decision":"oui"}`:                                    true,
	}
	for texte, attendu := range cas {
		d, err := analyserDecision(texte)
		if err != nil || d.Accepte != attendu {
			t.Errorf("%q → %+v, %v", texte, d, err)
		}
	}
	for _, texte := range []string{"", "{}", `{"decision":"peut-être"}`, "{pas du json}"} {
		if _, err := analyserDecision(texte); err == nil {
			t.Errorf("%q aurait dû être rejeté", texte)
		}
	}
}

// Une simulation complète avec LLM (simulé) fonctionne et reste sûre en concurrence
func TestSimulationAvecLLM(t *testing.T) {
	srv := fauxOllama(t, `{"decision":"accepte","raison":"coopérer"}`, http.StatusOK)
	defer srv.Close()
	j := NouveauJournalLLM(nil)
	cfg := ConfigParDefaut()
	cfg.TicksParJour = 60
	m := NouveauMonde(cfg, FabriqueLLM(NouveauClientLLM(srv.URL, "test", 2*time.Second, 4), j))
	for m.JoursTermines() < 2 && m.NbAgents() > 0 {
		m.Pas()
	}
	if strings.HasPrefix(j.Resume(), "aucun") {
		t.Fatalf("le LLM n'a jamais été sollicité")
	}
}
