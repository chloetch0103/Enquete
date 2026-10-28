package sma

import (
	"fmt"
	"math"
)

// Pos est une position (ou un vecteur) dans le plan.
type Pos struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Distance euclidienne entre deux positions
func (p Pos) Distance(q Pos) float64 { return math.Hypot(p.X-q.X, p.Y-q.Y) }

// Vers renvoie la position atteinte en avançant d'au plus 'pas' vers 'cible'
func (p Pos) Vers(cible Pos, pas float64) Pos {
	d := p.Distance(cible)
	if d <= pas {
		return cible
	}
	return Pos{p.X + (cible.X-p.X)/d*pas, p.Y + (cible.Y-p.Y)/d*pas}
}

// Borner ramène la position à l'intérieur du rectangle [0,largeur]×[0,hauteur].
func (p Pos) Borner(largeur, hauteur float64) Pos {
	return Pos{math.Max(0, math.Min(largeur, p.X)), math.Max(0, math.Min(hauteur, p.Y))}
}

// Dans indique si la position est à l'intérieur du monde.
func (p Pos) Dans(largeur, hauteur float64) bool {
	return p.X >= 0 && p.X <= largeur && p.Y >= 0 && p.Y <= hauteur
}

// TypeAgent : le "caractère" social d'un agent.
type TypeAgent int

const (
	Altruiste TypeAgent = iota
	Neutre
	Egoiste
)

// TypesAgents liste les types dans l'ordre des tableaux de statistiques.
var TypesAgents = []TypeAgent{Altruiste, Neutre, Egoiste}

func (t TypeAgent) String() string {
	switch t {
	case Altruiste:
		return "altruiste"
	case Neutre:
		return "neutre"
	case Egoiste:
		return "egoiste"
	}
	return "inconnu"
}

// MarshalText fait apparaître le nom (et non l'entier) dans le JSON.
func (t TypeAgent) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// TypeObjet : ressource présente dans l'environnement.
type TypeObjet int

const (
	Pomme TypeObjet = iota // se mange seul
	Vache                  // nécessite deux agents
)

func (t TypeObjet) String() string {
	if t == Vache {
		return "vache"
	}
	return "pomme"
}

func (t TypeObjet) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// Objet est une ressource consommable.
type Objet struct {
	ID      int       `json:"id"`
	Type    TypeObjet `json:"type"`
	Pos     Pos       `json:"pos"`
	Energie int       `json:"energie"`
}

// Etat de l'agent dans le protocole de coordination.
type Etat int

const (
	Libre               Etat = iota // cherche des pommes ou une vache
	AttenteReponses                 // a proposé une chasse, attend les réponses
	AttenteConfirmation             // a accepté une proposition, attend la confirmation
	Engage                          // binôme formé, se dirige vers la vache
)

func (e Etat) String() string {
	switch e {
	case AttenteReponses:
		return "attente-reponses"
	case AttenteConfirmation:
		return "attente-confirmation"
	case Engage:
		return "engage"
	}
	return "libre"
}

func (e Etat) MarshalText() ([]byte, error) { return []byte(e.String()), nil }

// UnmarshalText permet de relire le JSON produit
func (t *TypeAgent) UnmarshalText(b []byte) error {
	for _, c := range TypesAgents {
		if c.String() == string(b) {
			*t = c
			return nil
		}
	}
	return fmt.Errorf("type d'agent inconnu %q", b)
}

func (t *TypeObjet) UnmarshalText(b []byte) error {
	for _, c := range []TypeObjet{Pomme, Vache} {
		if c.String() == string(b) {
			*t = c
			return nil
		}
	}
	return fmt.Errorf("type d'objet inconnu %q", b)
}

func (e *Etat) UnmarshalText(b []byte) error {
	for _, c := range []Etat{Libre, AttenteReponses, AttenteConfirmation, Engage} {
		if c.String() == string(b) {
			*e = c
			return nil
		}
	}
	return fmt.Errorf("état inconnu %q", b)
}
