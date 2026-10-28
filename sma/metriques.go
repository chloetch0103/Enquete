package sma

import (
	"encoding/csv"
	"io"
	"strconv"
)

// EcrireCSV exporte l'historique journalier sous former d'un csv
func EcrireCSV(w io.Writer, historique []StatsJour) error {
	cw := csv.NewWriter(w)
	entete := []string{"jour"}
	for _, prefixe := range []string{"population", "morts", "naissances"} {
		for _, t := range TypesAgents {
			entete = append(entete, prefixe+"_"+t.String())
		}
	}
	entete = append(entete, "pommes_mangees", "vaches_partagees", "vaches_gachees", "messages")
	if err := cw.Write(entete); err != nil {
		return err
	}
	for _, s := range historique {
		ligne := []string{strconv.Itoa(s.Jour)}
		for _, tab := range [][3]int{s.Population, s.Morts, s.Naissances} {
			for _, v := range tab {
				ligne = append(ligne, strconv.Itoa(v))
			}
		}
		for _, v := range []int{s.PommesMangees, s.VachesPartagees, s.VachesGachees, s.Messages} {
			ligne = append(ligne, strconv.Itoa(v))
		}
		if err := cw.Write(ligne); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
