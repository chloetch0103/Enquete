// Package sma implémente une simulation multi-agents (SMA) de coopération :
// des agents altruistes, neutres et égoïstes ramassent des pommes seuls et
// doivent se coordonner à deux, par messages, pour chasser des vaches.
//
// Architecture :
//
//	perception (séquentielle) --> délibération (parallèle, une goroutine par agent)
//	--> réaction de l'environnement (séquentielle, ordre aléatoire reproductible)
//
// Les agents ne modifient jamais directement le monde : ils renvoient des
// "influences" (déplacement voulu, pomme à manger, messages à envoyer) que
// l'environnement valide et applique. Cela supprime les accès concurrents comme dans la première version.
package sma

// Config regroupe tous les paramètres de la simulation
type Config struct {
	Largeur, Hauteur float64 // dimensions du monde

	NbAltruistes, NbNeutres, NbEgoistes int // population initiale
	PopulationMax                       int // capacité de charge du milieu

	ChampDeVision float64 // rayon de perception d'un agent
	Vitesse       float64 // distance max parcourue par tick
	Portee        float64 // distance à laquelle on peut consommer un objet

	TicksParJour      int // durée d'une journée
	CoutJournalier    int // énergie consommée chaque soir ; en dessous, l'agent meurt
	SeuilReproduction int // énergie restante (après coût) nécessaire pour avoir un enfant

	ObjetsInitiaux  int
	MaxObjets       int
	ProbaApparition float64 // probabilité qu'un objet apparaisse à chaque tick
	ProbaVache      float64 // part des objets qui sont des vaches
	EnergiePomme    int
	EnergieVache    int

	ProbaProposer      float64 // probabilité qu'un agent libre lance une négociation en voyant une vache
	MaxDestinataires   int     // nombre de voisins sollicités par proposition
	DelaiReponse       int     // ticks d'attente max d'une réponse ou d'une confirmation
	DureeMaxEngagement int     // ticks max pour rejoindre la vache une fois engagés

	Seed int64 // graine : même graine ==> même simulation (sans LLM)
}

// ConfigParDefaut renvoie un jeu de paramètres calibré pour que les trois
// populations coexistent quelques jours avant que la sélection opère.
func ConfigParDefaut() Config {
	return Config{
		Largeur: 1000, Hauteur: 700,
		NbAltruistes: 10, NbNeutres: 10, NbEgoistes: 10,
		PopulationMax: 150,

		ChampDeVision: 120,
		Vitesse:       6,
		Portee:        8,

		TicksParJour:      300,
		CoutJournalier:    5,
		SeuilReproduction: 8,

		ObjetsInitiaux:  30,
		MaxObjets:       40,
		ProbaApparition: 0.45,
		ProbaVache:      0.35,
		EnergiePomme:    3,
		EnergieVache:    14,

		ProbaProposer:      0.3,
		MaxDestinataires:   3,
		DelaiReponse:       6,
		DureeMaxEngagement: 150,

		Seed: 42,
	}
}
