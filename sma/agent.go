package sma

import (
	"math"
	"math/rand"
	"sort"
)

// Agent : entité autonome. Ses champs publics (Pos, Energie) ne sont modifiés
// que par l'environnement pendant la phase de réaction ; son état interne
// (protocole) n'est modifié que par lui-même pendant sa délibération.
// Aucun verrou n'est donc nécessaire.

type Agent struct {
	ID      int
	Type    TypeAgent
	Pos     Pos
	Energie int

	strategie Strategie
	cfg       *Config
	rng       *rand.Rand // générateur propre ==> délibération parallèle reproductible
	cap       float64    // direction de la marche aléatoire

	// État du protocole de coordination
	etat          Etat
	depuis        int     // tick d'entrée dans l'état courant
	vacheID       int     // vache visée (0 si aucune)
	partenaire    int     // ID du partenaire (0 si aucun)
	revendication float64 // part que l'agent réclamera au partage
	enAttente     int     // nombre de réponses encore attendues (proposant)
}

// Etat renvoie l'état de l'agent dans le protocole.
func (a *Agent) Etat() Etat { return a.etat }

// VueAgent : ce qu'un agent perçoit d'un autre.
type VueAgent struct {
	ID    int
	Type  TypeAgent
	Pos   Pos
	Libre bool
}

// Perception : tout ce que l'environnement transmet à l'agent pour un tick.
// C'est une copie : l'agent ne peut pas modifier le monde à travers elle.
type Perception struct {
	Tick              int
	TicksAvantFinJour int        // nombres de Ticks restants avant la fin de la journée en cours
	Objets            []Objet    // objets dans le champ de vision
	Voisins           []VueAgent // agents dans le champ de vision
	Boite             []Message  // messages reçus
	Vache             *Objet     // vache visée par l'agent si elle existe encore
}

// Influence : ce que l'agent voudrait faire. L'environnement décide.
type Influence struct {
	Destination Pos // la position où l'agent veut se trouver à la fin du Tick
	Manger      int // ID de la pomme à manger, 0 sinon
	Messages    []Message
}

func (a *Agent) liberer(tick int) {
	a.etat, a.depuis = Libre, tick
	a.vacheID, a.partenaire, a.revendication, a.enAttente = 0, 0, 0, 0
}

// Decider : délibération de l'agent (exécutée en parallèle des autres agents).
func (a *Agent) Decider(p Perception) Influence {
	inf := Influence{Destination: a.Pos}
	envoyer := func(m Message) {
		m.De, m.Tick = a.ID, p.Tick
		inf.Messages = append(inf.Messages, m)
	}

	// 1. Messages de suivi du protocole (les propositions sont traitées ensuite,
	//    pour qu'un agent qui vient de s'engager les refuse).
	var propositions []Message
	for _, m := range p.Boite {
		switch m.Perf {
		case Proposition:
			propositions = append(propositions, m)

		case Acceptation:
			if a.etat == AttenteReponses && m.VacheID == a.vacheID {
				// Premier qui accepte : on forme le binôme.
				a.etat, a.depuis, a.partenaire = Engage, p.Tick, m.De
				a.revendication = a.strategie.Demande()
				envoyer(Message{A: m.De, Perf: Confirmation, VacheID: m.VacheID})
			} else {
				envoyer(Message{A: m.De, Perf: Annulation, VacheID: m.VacheID, Raison: "binôme déjà formé"})
			}

		case Refus:
			if a.etat == AttenteReponses && m.VacheID == a.vacheID {
				a.enAttente--
				if a.enAttente <= 0 {
					a.liberer(p.Tick) // tout le monde a refusé
				}
			}

		case Confirmation:
			if a.etat == AttenteConfirmation && m.De == a.partenaire && m.VacheID == a.vacheID {
				a.etat, a.depuis = Engage, p.Tick
			} else {
				// Confirmation arrivée trop tard : on prévient pour ne pas bloquer l'autre.
				envoyer(Message{A: m.De, Perf: Annulation, VacheID: m.VacheID, Raison: "plus disponible"})
			}

		case Annulation:
			if (a.etat == AttenteConfirmation || a.etat == Engage) && m.De == a.partenaire {
				a.liberer(p.Tick)
			}
		}
	}

	// 2. Propositions : on examine d'abord les plus généreuses
	//    (généralise la priorité altruiste > neutre > égoïste de la v1).
	if len(propositions) > 0 {
		sort.SliceStable(propositions, func(i, j int) bool {
			if propositions[i].Demande != propositions[j].Demande {
				return propositions[i].Demande < propositions[j].Demande
			}
			return propositions[i].De < propositions[j].De
		})
		pommes := compter(p.Objets, Pomme)
		for _, m := range propositions {
			// Propositions croisées (A et B se proposent la même vache au même
			// moment) : sans règle, les deux refusent "occupé" indéfiniment
			// (livelock). L'agent d'identifiant le plus grand abandonne la sienne.
			if a.etat == AttenteReponses && m.VacheID == a.vacheID && a.ID > m.De {
				a.liberer(p.Tick)
			}
			if a.etat != Libre {
				envoyer(Message{A: m.De, Perf: Refus, VacheID: m.VacheID, Raison: "occupé"})
				continue
			}
			energieVache := 0
			for _, o := range p.Objets {
				if o.ID == m.VacheID {
					energieVache = o.Energie
				}
			}
			if energieVache == 0 {
				energieVache = a.cfg.EnergieVache // vache hors de mon champ de vision
			}
			ctx := ContexteNegociation{
				Tick: p.Tick, Moi: a.Type, MonEnergie: a.Energie,
				CoutJournalier: a.cfg.CoutJournalier, TicksAvantFinJour: p.TicksAvantFinJour,
				PommesVisibles: pommes, Proposant: m.De, EnergieVache: energieVache,
				DemandeProposant: m.Demande, MaRevendication: a.strategie.Revendication(m.Demande),
			}
			d := a.strategie.Repondre(ctx)
			if d.Accepte {
				a.etat, a.depuis, a.partenaire, a.vacheID = AttenteConfirmation, p.Tick, m.De, m.VacheID
				a.revendication = ctx.MaRevendication
				envoyer(Message{A: m.De, Perf: Acceptation, VacheID: m.VacheID, Raison: d.Raison})
			} else {
				envoyer(Message{A: m.De, Perf: Refus, VacheID: m.VacheID, Raison: d.Raison})
			}
		}
	}

	// 3. Délais et abandons.
	switch a.etat {
	case AttenteReponses, AttenteConfirmation:
		if p.Vache == nil || p.Tick-a.depuis > a.cfg.DelaiReponse { //attente de réponse dépassé ou il y a plus de vache
			a.liberer(p.Tick)
		}
	case Engage:
		if p.Vache == nil { // mangée par un autre binôme
			a.liberer(p.Tick)
		} else if p.Tick-a.depuis > a.cfg.DureeMaxEngagement {
			envoyer(Message{A: a.partenaire, Perf: Annulation, VacheID: a.vacheID, Raison: "trop long"})
			a.liberer(p.Tick)
		}
	}

	// 4. Action physique.
	switch a.etat {
	case Engage:
		inf.Destination = a.Pos.Vers(p.Vache.Pos, a.cfg.Vitesse)
		return inf
	case AttenteReponses, AttenteConfirmation:
		return inf // on attend sur place
	}

	// Agent libre : vache visible et voisins disponibles --> on propose une chasse.
	if vache := plusProche(p.Objets, Vache, a.Pos); vache != nil && a.rng.Float64() < a.cfg.ProbaProposer {
		libres := voisinsLibres(p.Voisins, a.Pos, a.cfg.MaxDestinataires)
		if len(libres) > 0 {
			a.etat, a.depuis, a.vacheID, a.enAttente = AttenteReponses, p.Tick, vache.ID, len(libres)
			for _, v := range libres {
				envoyer(Message{A: v.ID, Perf: Proposition, VacheID: vache.ID, Demande: a.strategie.Demande()})
			}
			return inf
		}
	}

	// Sinon, la pomme la plus proche.
	if pomme := plusProche(p.Objets, Pomme, a.Pos); pomme != nil {
		inf.Destination = a.Pos.Vers(pomme.Pos, a.cfg.Vitesse)
		if inf.Destination.Distance(pomme.Pos) <= a.cfg.Portee {
			inf.Manger = pomme.ID
		}
		return inf
	}

	// Rien en vue : marche aléatoire
	a.cap += (a.rng.Float64() - 0.5) * 0.8
	dest := Pos{a.Pos.X + math.Cos(a.cap)*a.cfg.Vitesse, a.Pos.Y + math.Sin(a.cap)*a.cfg.Vitesse}
	if !dest.Dans(a.cfg.Largeur, a.cfg.Hauteur) {
		a.cap += math.Pi
		dest = Pos{a.Pos.X + math.Cos(a.cap)*a.cfg.Vitesse, a.Pos.Y + math.Sin(a.cap)*a.cfg.Vitesse}
	}
	inf.Destination = dest
	return inf
}

func plusProche(objets []Objet, t TypeObjet, depuis Pos) *Objet {
	var meilleur *Objet
	dMin := math.Inf(1)
	for i := range objets {
		if objets[i].Type != t {
			continue
		}
		if d := depuis.Distance(objets[i].Pos); d < dMin {
			dMin, meilleur = d, &objets[i]
		}
	}
	return meilleur
}

func voisinsLibres(voisins []VueAgent, depuis Pos, max int) []VueAgent {
	var libres []VueAgent
	for _, v := range voisins {
		if v.Libre {
			libres = append(libres, v)
		}
	}
	sort.SliceStable(libres, func(i, j int) bool { // On trie par en fontion des agents les plus proches
		return depuis.Distance(libres[i].Pos) < depuis.Distance(libres[j].Pos)
	})
	if len(libres) > max {
		libres = libres[:max]
	}
	return libres
}

func compter(objets []Objet, t TypeObjet) int {
	n := 0
	for _, o := range objets {
		if o.Type == t {
			n++
		}
	}
	return n
}
