package sma

import "sync"

// Performatif : l'intention d'un message

type Performatif int

const (
	Proposition Performatif = iota
	Acceptation
	Refus
	Confirmation
	Annulation
)

func (p Performatif) String() string {
	switch p {
	case Proposition:
		return "PROPOSITION"
	case Acceptation:
		return "ACCEPTATION"
	case Refus:
		return "REFUS"
	case Confirmation:
		return "CONFIRMATION"
	case Annulation:
		return "ANNULATION"
	}
	return "?"
}

func (p Performatif) MarshalText() ([]byte, error) { return []byte(p.String()), nil }

// Message échangé entre agents. Un message envoyé au tick t est lu au tick t+1.
type Message struct {
	De      int         `json:"de"`
	A       int         `json:"a"`
	Perf    Performatif `json:"performatif"`
	VacheID int         `json:"vache"`
	Demande float64     `json:"demande,omitempty"` // part réclamée par le proposant
	Raison  string      `json:"raison,omitempty"`  // justification (traçabilité des décisions)
	Tick    int         `json:"tick"`
}

// Bus : boîtes aux lettres des agents, sûres en accès concurrent.
type Bus struct {
	mu     sync.Mutex
	boites map[int][]Message
}

func NouveauBus() *Bus { return &Bus{boites: make(map[int][]Message)} }

// Envoyer dépose un message dans la boîte du destinataire.
func (b *Bus) Envoyer(m Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.boites[m.A] = append(b.boites[m.A], m)
}

// Relever renvoie et vide la boîte aux lettres d'un agent.
func (b *Bus) Relever(id int) []Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	msgs := b.boites[id]
	delete(b.boites, id)
	return msgs
}

// Vider supprime tous les messages en attente (fin de journée).
func (b *Bus) Vider() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.boites = make(map[int][]Message)
}
