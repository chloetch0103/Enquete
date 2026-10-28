package sma

import (
	"context"
	"sync/atomic"
	"time"
)

// VueAgentEtat : représentation d'un agent pour l'affichage.
type VueAgentEtat struct {
	ID         int       `json:"id"`
	Type       TypeAgent `json:"type"`
	Pos        Pos       `json:"pos"`
	Energie    int       `json:"energie"`
	Etat       Etat      `json:"etat"`
	Partenaire int       `json:"partenaire,omitempty"`
	VacheID    int       `json:"vache,omitempty"`
}

// Instantane : copie immuable de l'état du monde, publiée après chaque tick.
type Instantane struct {
	Tick          int            `json:"tick"`
	Jour          int            `json:"jour"`
	TickDansJour  int            `json:"tick_dans_jour"`
	TicksParJour  int            `json:"ticks_par_jour"`
	Largeur       float64        `json:"largeur"`
	Hauteur       float64        `json:"hauteur"`
	ChampDeVision float64        `json:"champ_de_vision"`
	EnPause       bool           `json:"en_pause"`
	Agents        []VueAgentEtat `json:"agents"`
	Objets        []Objet        `json:"objets"`
	Courant       StatsJour      `json:"courant"`
	Historique    []StatsJour    `json:"historique"`
	Journal       []Evenement    `json:"journal"`
}

// Instantane construit une copie profonde de l'état courant.
func (m *Monde) Instantane() *Instantane {
	in := &Instantane{
		Tick: m.tick, Jour: m.jour + 1, TickDansJour: m.tickDansJour, TicksParJour: m.cfg.TicksParJour,
		Largeur: m.cfg.Largeur, Hauteur: m.cfg.Hauteur, ChampDeVision: m.cfg.ChampDeVision,
		Agents:     make([]VueAgentEtat, len(m.agents)),
		Objets:     make([]Objet, len(m.objets)),
		Courant:    m.stats,
		Historique: append([]StatsJour(nil), m.historique...),
		Journal:    append([]Evenement(nil), m.journal...),
	}
	for i, a := range m.agents {
		in.Agents[i] = VueAgentEtat{ID: a.ID, Type: a.Type, Pos: a.Pos, Energie: a.Energie,
			Etat: a.etat, Partenaire: a.partenaire, VacheID: a.vacheID}
	}
	for i, o := range m.objets {
		in.Objets[i] = *o
	}
	in.Courant.Population = m.population()
	return in
}

// Pilote fait tourner le monde dans sa propre goroutine. Les autres goroutines
// (serveur HTTP) n'y accèdent jamais directement : elles lisent le dernier
// instantané publié (atomic.Pointer, sans verrou) et envoient des commandes
// par canal
type Pilote struct {
	monde      *Monde
	nouveau    func() *Monde
	commandes  chan func()
	instantane atomic.Pointer[Instantane]
	enPause    atomic.Bool
	delai      atomic.Int64
}

// NouveauPilote : nouveau() sert à (ré)initialiser le monde.
func NouveauPilote(nouveau func() *Monde, delai time.Duration) *Pilote {
	p := &Pilote{monde: nouveau(), nouveau: nouveau, commandes: make(chan func(), 8)}
	p.delai.Store(int64(delai))
	p.enPause.Store(true)
	p.publier()
	return p
}

// Executer : boucle de simulation, jusqu'à l'annulation du contexte.
func (p *Pilote) Executer(ctx context.Context) {
	for {
		attente := time.Duration(p.delai.Load())
		if p.enPause.Load() {
			attente = 50 * time.Millisecond
		}
		timer := time.NewTimer(attente)
		select {
		case <-ctx.Done(): // demande d'arrêt du programme
			timer.Stop()
			return
		case cmd := <-p.commandes: // une commande arrive du tableau de bord
			timer.Stop()
			cmd()
			p.publier()
			continue
		case <-timer.C: // le délai est écoulé
		}
		if !p.enPause.Load() && p.monde.NbAgents() > 0 {
			p.monde.Pas()
			p.publier()
		}
	}
}

func (p *Pilote) publier() {
	in := p.monde.Instantane()
	in.EnPause = p.enPause.Load()
	p.instantane.Store(in)
}

// Dernier renvoie le dernier instantané publié (ne bloque jamais).
func (p *Pilote) Dernier() *Instantane { return p.instantane.Load() }

func (p *Pilote) Pause(pause bool) {
	p.enPause.Store(pause)
	p.commandes <- func() {}
}

func (p *Pilote) Delai(d time.Duration) { p.delai.Store(int64(d)) }

// JourSuivant simule d'un coup les ticks restants de la journée en cours.
func (p *Pilote) JourSuivant() { p.commandes <- func() { p.monde.TerminerJournee() } }

// Reinitialiser recrée un monde neuf.
func (p *Pilote) Reinitialiser() { p.commandes <- func() { p.monde = p.nouveau() } }
