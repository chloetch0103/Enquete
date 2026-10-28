package sma

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
)

// StatsJour : indicateurs agrégés sur une journée (index des tableaux = TypeAgent).
type StatsJour struct {
	Jour            int    `json:"jour"`
	Population      [3]int `json:"population"`
	Morts           [3]int `json:"morts"`
	Naissances      [3]int `json:"naissances"`
	PommesMangees   int    `json:"pommes_mangees"`
	VachesPartagees int    `json:"vaches_partagees"`
	VachesGachees   int    `json:"vaches_gachees"`
	Messages        int    `json:"messages"`
}

// Evenement : trace lisible d'une interaction (journal affiché dans l'interface)
type Evenement struct {
	Tick  int    `json:"tick"`
	Texte string `json:"texte"`
}

const tailleJournal = 60

// Monde : l'environnement. Il n'est pas protégé par un verrou : une seule
// goroutine (le Pilote, ou le mode sans interface) le fait avancer.
// La seule concurrence interne est la délibération parallèle des agents,
// qui ne lisent que leur Perception.
type Monde struct {
	cfg      Config
	rng      *rand.Rand
	fabrique FabriqueStrategie
	bus      *Bus

	tick, tickDansJour, jour int
	prochainID               int

	agents []*Agent
	objets []*Objet

	stats      StatsJour   // journée en cours
	historique []StatsJour // journées terminées (le jour 0 = population initiale)
	journal    []Evenement
}

// NouveauMonde crée un monde peuplé selon cfg. fabrique peut être nil
func NouveauMonde(cfg Config, fabrique FabriqueStrategie) *Monde {
	if fabrique == nil {
		fabrique = NouvelleStrategie
	}
	m := &Monde{cfg: cfg, rng: rand.New(rand.NewSource(cfg.Seed)), fabrique: fabrique, bus: NouveauBus()}
	nb := map[TypeAgent]int{Altruiste: cfg.NbAltruistes, Neutre: cfg.NbNeutres, Egoiste: cfg.NbEgoistes}
	// Ajout des agents initialement présents dans le monde
	for _, t := range TypesAgents {
		for i := 0; i < nb[t]; i++ {
			m.AjouterAgent(t, m.positionAleatoire())
		}
	}
	// Ajout des objets
	for i := 0; i < cfg.ObjetsInitiaux; i++ {
		m.apparition()
	}
	m.historique = []StatsJour{{Jour: 0, Population: m.population()}}
	m.stats = StatsJour{Jour: 1}
	return m
}

// AjouterAgent ajoute un agent (initialisation, naissances, tests).
func (m *Monde) AjouterAgent(t TypeAgent, p Pos) *Agent {
	m.prochainID++
	a := &Agent{
		ID: m.prochainID, Type: t, Pos: p,
		strategie: m.fabrique(t), cfg: &m.cfg,
		rng: rand.New(rand.NewSource(m.rng.Int63())),
		cap: m.rng.Float64() * 2 * math.Pi,
	}
	m.agents = append(m.agents, a)
	return a
}

// AjouterObjet ajoute une ressource à une position donnée.
func (m *Monde) AjouterObjet(t TypeObjet, p Pos) *Objet {
	m.prochainID++
	e := m.cfg.EnergiePomme
	if t == Vache {
		e = m.cfg.EnergieVache
	}
	o := &Objet{ID: m.prochainID, Type: t, Pos: p, Energie: e}
	m.objets = append(m.objets, o)
	return o
}

// Pas fait avancer la simulation d'un tick (cycle influence/réaction)
func (m *Monde) Pas() {
	m.tick++
	m.tickDansJour++
	n := len(m.agents)

	// 1. Perception (séquentielle, lecture seule du monde)
	perceptions := make([]Perception, n)
	for i, a := range m.agents {
		perceptions[i] = m.percevoir(a)
	}

	// 2. Délibération en parallèle : chaque agent ne touche qu'à son propre état.
	influences := make([]Influence, n)
	var wg sync.WaitGroup
	for i, a := range m.agents {
		wg.Add(1)
		go func(i int, a *Agent) {
			defer wg.Done()
			influences[i] = a.Decider(perceptions[i])
		}(i, a)
	}
	wg.Wait()

	// 3. Réaction : l'environnement applique les influences dans un ordre
	//    aléatoire (équitable) mais reproductible, et arbitre les conflits
	for _, i := range m.rng.Perm(n) {
		m.appliquer(m.agents[i], influences[i])
	}
	m.resoudreChasses()
	if len(m.objets) < m.cfg.MaxObjets && m.rng.Float64() < m.cfg.ProbaApparition {
		m.apparition()
	}

	if m.tickDansJour >= m.cfg.TicksParJour {
		m.FinDeJournee()
	}
}

func (m *Monde) percevoir(a *Agent) Perception {
	p := Perception{
		Tick:              m.tick,
		TicksAvantFinJour: m.cfg.TicksParJour - m.tickDansJour,
		Boite:             m.bus.Relever(a.ID),
	}
	for _, o := range m.objets {
		if a.Pos.Distance(o.Pos) <= m.cfg.ChampDeVision {
			p.Objets = append(p.Objets, *o)
		}
		// Cas où l'agent reçoit une proposition hors de son champs de vision
		if a.etat != Libre && o.ID == a.vacheID {
			v := *o
			p.Vache = &v
		}
	}
	for _, b := range m.agents {
		if b != a && a.Pos.Distance(b.Pos) <= m.cfg.ChampDeVision {
			p.Voisins = append(p.Voisins, VueAgent{ID: b.ID, Type: b.Type, Pos: b.Pos, Libre: b.etat == Libre})
		}
	}
	return p
}

func (m *Monde) appliquer(a *Agent, inf Influence) {
	// Physique : vitesse maximale et bords du monde imposés par l'environnement.
	a.Pos = a.Pos.Vers(inf.Destination, m.cfg.Vitesse).Borner(m.cfg.Largeur, m.cfg.Hauteur)

	for _, msg := range inf.Messages {
		msg.De = a.ID // l'environnement garantit l'identité de l'expéditeur
		m.bus.Envoyer(msg)
		m.stats.Messages++
		m.tracerMessage(msg)
	}

	if inf.Manger != 0 {
		// La pomme a pu être mangée par un agent passé avant : premier arrivé, premier servi.
		if i := m.indexObjet(inf.Manger); i >= 0 {
			o := m.objets[i]
			if o.Type == Pomme && a.Pos.Distance(o.Pos) <= m.cfg.Portee {
				a.Energie += o.Energie
				m.retirerObjet(i)
				m.stats.PommesMangees++
			}
		}
	}
}

// resoudreChasses : une vache est consommée quand les deux membres d'un
// binôme sont à portée. Le partage suit le jeu de négociation de Nash
func (m *Monde) resoudreChasses() {
	parID := make(map[int]*Agent, len(m.agents))
	for _, a := range m.agents {
		parID[a.ID] = a
	}
	for i := 0; i < len(m.objets); i++ {
		v := m.objets[i]
		if v.Type != Vache {
			continue
		}
		for _, a := range m.agents {
			if a.etat != Engage || a.vacheID != v.ID || a.Pos.Distance(v.Pos) > m.cfg.Portee {
				continue
			}
			b := parID[a.partenaire]
			if b == nil || b.etat != Engage || b.partenaire != a.ID || b.vacheID != v.ID ||
				b.Pos.Distance(v.Pos) > m.cfg.Portee {
				continue
			}
			if a.revendication+b.revendication <= 1+1e-9 {
				ga := int(math.Round(float64(v.Energie) * a.revendication))
				gb := int(math.Round(float64(v.Energie) * b.revendication))
				a.Energie += ga
				b.Energie += gb
				m.stats.VachesPartagees++
				m.tracer(fmt.Sprintf("vache#%d partagée : %s#%d +%d, %s#%d +%d",
					v.ID, a.Type, a.ID, ga, b.Type, b.ID, gb))
			} else {
				m.stats.VachesGachees++
				m.tracer(fmt.Sprintf("vache#%d gâchée : %s#%d réclame %.0f %%, %s#%d réclame %.0f %%",
					v.ID, a.Type, a.ID, a.revendication*100, b.Type, b.ID, b.revendication*100))
			}
			a.liberer(m.tick)
			b.liberer(m.tick)
			m.retirerObjet(i)
			i--
			break
		}
	}
}

// FinDeJournee : sélection (les agents trop faibles meurent), reproduction,
// puis remise à zéro des énergies. Appelée automatiquement ou depuis l'interface.
func (m *Monde) FinDeJournee() {
	s := &m.stats
	var survivants, parents []*Agent
	for _, a := range m.agents {
		if a.Energie < m.cfg.CoutJournalier {
			s.Morts[a.Type]++
			continue
		}
		a.Energie -= m.cfg.CoutJournalier
		if a.Energie >= m.cfg.SeuilReproduction {
			parents = append(parents, a)
		}
		survivants = append(survivants, a)
	}
	m.agents = survivants
	for _, p := range parents {
		if len(m.agents) >= m.cfg.PopulationMax {
			break
		}
		m.AjouterAgent(p.Type, Pos{})
		s.Naissances[p.Type]++
	}
	for _, a := range m.agents {
		a.Energie = 0
		a.liberer(m.tick)
		a.Pos = m.positionAleatoire()
	}
	m.bus.Vider()

	s.Population = m.population()
	m.historique = append(m.historique, *s)
	m.tracer(fmt.Sprintf("fin du jour %d : %d altruistes, %d neutres, %d égoïstes",
		s.Jour, s.Population[Altruiste], s.Population[Neutre], s.Population[Egoiste]))
	m.jour++
	m.tickDansJour = 0
	m.stats = StatsJour{Jour: m.jour + 1}
}

// TerminerJournee avance jusqu'à la fin de la journée en cours.
func (m *Monde) TerminerJournee() {
	jour := m.jour
	for m.jour == jour && len(m.agents) > 0 {
		m.Pas()
	}
}

// --- Accès en lecture ---

func (m *Monde) Tick() int                 { return m.tick }
func (m *Monde) JoursTermines() int        { return m.jour }
func (m *Monde) NbAgents() int             { return len(m.agents) }
func (m *Monde) Config() Config            { return m.cfg }
func (m *Monde) Historique() []StatsJour   { return append([]StatsJour(nil), m.historique...) }
func (m *Monde) Agents() []*Agent          { return m.agents }
func (m *Monde) Objets() []*Objet          { return m.objets }
func (m *Monde) StatsCourantes() StatsJour { return m.stats }

// --- Utilitaires internes ---

// population sert à compter les agents par type
func (m *Monde) population() [3]int {
	var p [3]int
	for _, a := range m.agents {
		p[a.Type]++
	}
	return p
}

func (m *Monde) positionAleatoire() Pos {
	return Pos{m.rng.Float64() * m.cfg.Largeur, m.rng.Float64() * m.cfg.Hauteur}
}

func (m *Monde) apparition() {
	t := Pomme
	if m.rng.Float64() < m.cfg.ProbaVache {
		t = Vache
	}
	m.AjouterObjet(t, m.positionAleatoire())
}

func (m *Monde) indexObjet(id int) int {
	for i, o := range m.objets {
		if o.ID == id {
			return i
		}
	}
	return -1
}

// retirerObjet supprime l'objet d'indice i sans aliasing de tranche.
func (m *Monde) retirerObjet(i int) {
	copy(m.objets[i:], m.objets[i+1:])
	m.objets[len(m.objets)-1] = nil
	m.objets = m.objets[:len(m.objets)-1] // pour éviter que le dernier élément apparaisse en double
}

func (m *Monde) tracer(texte string) {
	m.journal = append(m.journal, Evenement{Tick: m.tick, Texte: texte})
	if len(m.journal) > tailleJournal {
		m.journal = m.journal[len(m.journal)-tailleJournal:]
	}
}

func (m *Monde) tracerMessage(msg Message) {
	txt := fmt.Sprintf("#%d --> #%d %s vache#%d", msg.De, msg.A, msg.Perf, msg.VacheID)
	if msg.Perf == Proposition {
		txt += fmt.Sprintf(" (demande %.0f %%)", msg.Demande*100)
	}
	if msg.Raison != "" {
		txt += " -- " + msg.Raison
	}
	m.tracer(txt)
}
