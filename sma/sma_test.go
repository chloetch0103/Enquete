package sma

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// mondeVide : aucun agent, aucun objet, aucune apparition aléatoire.
func mondeVide() *Monde {
	cfg := ConfigParDefaut()
	cfg.NbAltruistes, cfg.NbNeutres, cfg.NbEgoistes = 0, 0, 0
	cfg.ObjetsInitiaux, cfg.ProbaApparition = 0, 0
	cfg.ProbaProposer = 1
	return NouveauMonde(cfg, nil)
}

func avancer(m *Monde, n int) {
	for i := 0; i < n; i++ {
		m.Pas()
	}
}

func TestDistanceEtVers(t *testing.T) {
	a, b := Pos{1, 1}, Pos{2, 2}
	if got := a.Distance(b); math.Abs(got-math.Sqrt2) > 1e-12 {
		t.Fatalf("Distance = %v, attendu √2", got)
	}
	p := Pos{0, 0}.Vers(Pos{10, 0}, 3)
	if p != (Pos{3, 0}) {
		t.Fatalf("Vers = %v, attendu {3 0}", p)
	}
	if p := (Pos{0, 0}).Vers(Pos{1, 1}, 5); p != (Pos{1, 1}) {
		t.Fatalf("Vers ne s'arrête pas sur la cible : %v", p)
	}
	if p := (Pos{-5, 900}).Borner(100, 100); p != (Pos{0, 100}) {
		t.Fatalf("Borner = %v", p)
	}
}

func TestBusReleverVideLaBoite(t *testing.T) {
	b := NouveauBus()
	b.Envoyer(Message{De: 1, A: 2, Perf: Proposition})
	b.Envoyer(Message{De: 3, A: 2, Perf: Proposition})
	b.Envoyer(Message{De: 1, A: 4, Perf: Refus})
	if got := len(b.Relever(2)); got != 2 {
		t.Fatalf("Relever(2) = %d messages, attendu 2", got)
	}
	if got := len(b.Relever(2)); got != 0 {
		t.Fatalf("la boîte n'a pas été vidée (%d messages)", got)
	}
	if got := len(b.Relever(4)); got != 1 {
		t.Fatalf("Relever(4) = %d, attendu 1", got)
	}
}

func TestPommeMangeeUneSeuleFois(t *testing.T) {
	m := mondeVide()
	a := m.AjouterAgent(Neutre, Pos{100, 100})
	b := m.AjouterAgent(Neutre, Pos{110, 100})
	m.AjouterObjet(Pomme, Pos{105, 100})
	avancer(m, 3)
	if len(m.objets) != 0 {
		t.Fatalf("la pomme n'a pas été mangée")
	}
	if a.Energie+b.Energie != m.cfg.EnergiePomme {
		t.Fatalf("énergie totale = %d, attendu %d (une seule pomme)", a.Energie+b.Energie, m.cfg.EnergiePomme)
	}
}

// Protocole complet : proposition --> acceptation --> confirmation --> chasse --> partage
func TestChasseCooperative(t *testing.T) {
	m := mondeVide()
	alt := m.AjouterAgent(Altruiste, Pos{100, 100})
	neu := m.AjouterAgent(Neutre, Pos{130, 100})
	m.AjouterObjet(Vache, Pos{150, 150})
	avancer(m, 30)
	if len(m.objets) != 0 {
		t.Fatalf("la vache n'a pas été chassée")
	}
	if m.stats.VachesPartagees != 1 {
		t.Fatalf("VachesPartagees = %d, attendu 1", m.stats.VachesPartagees)
	}
	// Proposition croisée : l'agent #2 (neutre, ID le plus grand) cède et accepte
	// l'offre de l'altruiste (30 %), puis réclame 50 %.
	e := m.cfg.EnergieVache
	if alt.Energie != int(math.Round(0.3*float64(e))) || neu.Energie != int(math.Round(0.5*float64(e))) {
		t.Fatalf("partage inattendu : altruiste %d, neutre %d", alt.Energie, neu.Energie)
	}
	if alt.Etat() != Libre || neu.Etat() != Libre {
		t.Fatalf("les agents doivent être libérés après la chasse")
	}
}

// Deux égoïstes réclament 70 % chacun : la vache est perdue (jeu de Nash).
func TestConflitEntreEgoistes(t *testing.T) {
	m := mondeVide()
	a := m.AjouterAgent(Egoiste, Pos{100, 100})
	b := m.AjouterAgent(Egoiste, Pos{130, 100})
	m.AjouterObjet(Vache, Pos{150, 150})
	avancer(m, 30)
	if m.stats.VachesGachees != 1 || a.Energie != 0 || b.Energie != 0 {
		t.Fatalf("attendu une vache gâchée et 0 énergie, obtenu gâchées=%d, énergies %d/%d",
			m.stats.VachesGachees, a.Energie, b.Energie)
	}
}

// Un neutre refuse une offre inéquitable : aucun binôme ne se forme.
func TestNeutreRefuseOffreInequitable(t *testing.T) {
	s := NouvelleStrategie(Neutre)
	if d := s.Repondre(ContexteNegociation{DemandeProposant: 0.7}); d.Accepte {
		t.Fatalf("le neutre ne devrait pas accepter 70 %%")
	}
	if d := s.Repondre(ContexteNegociation{DemandeProposant: 0.5}); !d.Accepte {
		t.Fatalf("le neutre devrait accepter 50 %%")
	}
}

func TestFinDeJourneeSelectionEtReproduction(t *testing.T) {
	m := mondeVide()
	faible := m.AjouterAgent(Altruiste, Pos{})
	moyen := m.AjouterAgent(Neutre, Pos{})
	fort := m.AjouterAgent(Egoiste, Pos{})
	faible.Energie = m.cfg.CoutJournalier - 1
	moyen.Energie = m.cfg.CoutJournalier
	fort.Energie = m.cfg.CoutJournalier + m.cfg.SeuilReproduction
	m.FinDeJournee()

	h := m.Historique()
	dernier := h[len(h)-1]
	if dernier.Population != [3]int{0, 1, 2} {
		t.Fatalf("population = %v, attendu [0 1 2]", dernier.Population)
	}
	if dernier.Morts[Altruiste] != 1 || dernier.Naissances[Egoiste] != 1 {
		t.Fatalf("morts=%v naissances=%v", dernier.Morts, dernier.Naissances)
	}
	for _, a := range m.agents {
		if a.Energie != 0 {
			t.Fatalf("l'énergie doit être remise à zéro")
		}
	}
}

// Même graine ==> même histoire, malgré la délibération parallèle
func TestReproductibilite(t *testing.T) {
	cfg := ConfigParDefaut()
	cfg.TicksParJour = 100
	executer := func() []StatsJour {
		m := NouveauMonde(cfg, nil)
		for m.JoursTermines() < 5 && m.NbAgents() > 0 {
			m.Pas()
		}
		return m.Historique()
	}
	h1, h2 := executer(), executer()
	if len(h1) != len(h2) {
		t.Fatalf("longueurs différentes")
	}
	for i := range h1 {
		if h1[i] != h2[i] {
			t.Fatalf("jour %d différent :\n%+v\n%+v", i, h1[i], h2[i])
		}
	}
}

// Simulation longue sous le détecteur de courses (go test -race)
func TestSimulationComplete(t *testing.T) {
	cfg := ConfigParDefaut()
	cfg.TicksParJour = 100
	m := NouveauMonde(cfg, nil)
	for m.JoursTermines() < 5 && m.NbAgents() > 0 {
		m.Pas()
	}
	if m.JoursTermines() != 5 {
		t.Fatalf("simulation interrompue au jour %d", m.JoursTermines())
	}
	var buf bytes.Buffer
	if err := EcrireCSV(&buf, m.Historique()); err != nil {
		t.Fatal(err)
	}
	if lignes := strings.Count(buf.String(), "\n"); lignes != 7 { // en-tête + jours 0..5
		t.Fatalf("CSV : %d lignes, attendu 7", lignes)
	}
}
