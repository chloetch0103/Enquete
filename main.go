// Simulation multi-agents "Enquête" : coopération et négociation entre
// agents altruistes, neutres et égoïstes.
//
//	go run .                          tableau de bord web sur http://localhost:8080
//	go run . -headless -jours 40 -csv out.csv      simulation rapide, résumé + CSV
//	go run . -llm mistral             décisions de négociation par un LLM local (Ollama)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"Enquete/sma"
	"Enquete/web"
)

func main() {
	cfg := sma.ConfigParDefaut()
	flag.Int64Var(&cfg.Seed, "seed", cfg.Seed, "graine aléatoire")
	flag.IntVar(&cfg.NbAltruistes, "altruistes", cfg.NbAltruistes, "nombre initial d'altruistes")
	flag.IntVar(&cfg.NbNeutres, "neutres", cfg.NbNeutres, "nombre initial de neutres")
	flag.IntVar(&cfg.NbEgoistes, "egoistes", cfg.NbEgoistes, "nombre initial d'égoïstes")
	flag.IntVar(&cfg.TicksParJour, "ticks-par-jour", cfg.TicksParJour, "durée d'une journée en ticks")
	headless := flag.Bool("headless", false, "simulation sans interface")
	jours := flag.Int("jours", 30, "nombre de jours simulés (mode -headless)")
	fichierCSV := flag.String("csv", "", "fichier CSV de sortie (mode -headless)")
	adresse := flag.String("adresse", "localhost:8080", "adresse du tableau de bord web")
	modele := flag.String("llm", "", "modèle Ollama pour les négociations; vide = règles fixes")
	urlLLM := flag.String("llm-url", "http://localhost:11434", "URL du serveur Ollama")
	journalLLM := flag.String("llm-journal", "decisions_llm.jsonl", "journal des décisions du LLM")
	flag.Parse()

	var fabrique sma.FabriqueStrategie // nil = règles fixes
	if *modele != "" {
		f, err := os.Create(*journalLLM)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		client := sma.NouveauClientLLM(*urlLLM, *modele, 20*time.Second, 4)
		journal := sma.NouveauJournalLLM(f)
		fabrique = sma.FabriqueLLM(client, journal)
		log.Printf("Négociations confiées au modèle %q (%s), fichier de stockage du journal : %s", *modele, *urlLLM, *journalLLM)
		defer func() { log.Printf("LLM : %s", journal.Resume()) }()
	}

	if *headless {
		lancerSansInterface(cfg, fabrique, *jours, *fichierCSV)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	pilote := sma.NouveauPilote(func() *sma.Monde { return sma.NouveauMonde(cfg, fabrique) }, 30*time.Millisecond)
	go pilote.Executer(ctx)
	log.Printf("Tableau de bord : http://%s  (Ctrl+C pour quitter)", *adresse)
	if err := web.Servir(ctx, *adresse, pilote); err != nil {
		log.Fatal(err)
	}
}

func lancerSansInterface(cfg sma.Config, fabrique sma.FabriqueStrategie, jours int, fichierCSV string) {
	debut := time.Now()
	m := sma.NouveauMonde(cfg, fabrique)
	for m.JoursTermines() < jours && m.NbAgents() > 0 {
		m.Pas()
	}
	h := m.Historique()
	fmt.Printf("%4s  %9s %7s %8s  %6s %7s %7s %8s\n",
		"jour", "altruiste", "neutre", "égoïste", "pommes", "vaches+", "vaches-", "messages")
	for _, s := range h {
		fmt.Printf("%4d  %9d %7d %8d  %6d %7d %7d %8d\n", s.Jour,
			s.Population[sma.Altruiste], s.Population[sma.Neutre], s.Population[sma.Egoiste],
			s.PommesMangees, s.VachesPartagees, s.VachesGachees, s.Messages)
	}
	fmt.Printf("%d ticks simulés en %v\n", m.Tick(), time.Since(debut).Round(time.Millisecond))

	if fichierCSV != "" {
		f, err := os.Create(fichierCSV)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if err := sma.EcrireCSV(f, h); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Historique écrit dans", fichierCSV)
	}
}
