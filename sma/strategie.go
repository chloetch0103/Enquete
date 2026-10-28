package sma

import "fmt"

// ContexteNegociation : ce que l'agent sait quand il reçoit une proposition.
type ContexteNegociation struct {
	Tick              int
	Moi               TypeAgent
	MonEnergie        int
	CoutJournalier    int
	TicksAvantFinJour int
	PommesVisibles    int

	Proposant        int
	EnergieVache     int
	DemandeProposant float64 // part annoncée par le proposant
	MaRevendication  float64 // part que je réclamerai si j'accepte
}

// Decision prise par un agent, avec sa justification.
type Decision struct {
	Accepte bool
	Raison  string
}

// Strategie isole la prise de décision sociale de l'agent
// C'est le point d'extension : StrategieRegles (règles fixes) et
// StrategieLLM (LLM local) implémentent la même interface.
type Strategie interface {
	Type() TypeAgent
	// Demande : part de la vache annoncée quand l'agent propose une chasse
	Demande() float64
	// Repondre : accepter ou refuser une proposition de chasse
	Repondre(ctx ContexteNegociation) Decision
	// Revendication : part réellement réclamée au moment du partage
	Revendication(demandePartenaire float64) float64
}

// FabriqueStrategie crée la stratégie associée à un type d'agent
type FabriqueStrategie func(TypeAgent) Strategie

// StrategieRegles : comportements codés à la main.
//
// Le partage d'une vache est un "jeu de négociation de Nash" : chacun
// réclame une part. si la somme dépasse 100 %, personne ne mange et la vache disparaît.
//
//	           demande  accepte une proposition…   réclame ensuite
//	altruiste    30 %     toujours                   ce qui reste (1 - d)
//	neutre       50 %     si le proposant demande ≤ 50 %  50 %
//	égoïste      70 %     toujours                   70 %, quoi qu'il arrive
type StrategieRegles struct{ T TypeAgent }

// NouvelleStrategie est la fabrique par défaut (règles fixes).
func NouvelleStrategie(t TypeAgent) Strategie { return StrategieRegles{T: t} }

func (s StrategieRegles) Type() TypeAgent { return s.T }

func (s StrategieRegles) Demande() float64 {
	switch s.T {
	case Altruiste:
		return 0.3
	case Egoiste:
		return 0.7
	}
	return 0.5
}

func (s StrategieRegles) Repondre(ctx ContexteNegociation) Decision {
	d := ctx.DemandeProposant * 100
	switch s.T {
	case Altruiste:
		return Decision{true, fmt.Sprintf("je coopère et prends ce qui reste (%.0f %%)", 100-d)}
	case Neutre:
		if ctx.DemandeProposant <= 0.5+1e-9 {
			return Decision{true, fmt.Sprintf("offre équitable (%.0f %% demandés)", d)}
		}
		return Decision{false, fmt.Sprintf("offre inéquitable (%.0f %% demandés)", d)}
	default:
		return Decision{true, "j'accepte mais je réclamerai 70 %"}
	}
}

func (s StrategieRegles) Revendication(demandePartenaire float64) float64 {
	switch s.T {
	case Altruiste:
		r := 1 - demandePartenaire
		if r < 0 {
			r = 0
		}
		return r
	case Egoiste:
		return 0.7
	}
	return 0.5
}
