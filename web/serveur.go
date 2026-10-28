// Package web expose la simulation via une petite API HTTP/JSON et une page
// de supervision (HTML + canvas, sans dépendance externe)
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"time"

	"Enquete/sma"
)

//go:embed static
var statique embed.FS

type commande struct {
	Action  string `json:"action"` // pause | reprendre | jour | reinit | vitesse
	DelaiMs int    `json:"delai_ms,omitempty"`
}

// Routeur construit le gestionnaire HTTP
func Routeur(p *sma.Pilote) http.Handler {
	mux := http.NewServeMux()
	fichiers, _ := fs.Sub(statique, "static")
	mux.Handle("GET /", http.FileServerFS(fichiers))

	// État courant du monde (lu sans verrou depuis le dernier instantané publié).
	mux.HandleFunc("GET /api/etat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store") // pour ne pas garder la réponse en cache et ainsi éviter d'afficher un vieille état
		_ = json.NewEncoder(w).Encode(p.Dernier())
	})

	// Historique journalier au format CSV.
	mux.HandleFunc("GET /api/historique.csv", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="historique.csv"`) //pour télécharger le fichier csv
		_ = sma.EcrireCSV(w, p.Dernier().Historique)
	})

	// Commandes de supervision
	mux.HandleFunc("POST /api/commande", func(w http.ResponseWriter, r *http.Request) {
		var c commande
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&c); err != nil {
			http.Error(w, "JSON invalide", http.StatusBadRequest)
			return
		}
		switch c.Action {
		case "pause":
			p.Pause(true)
		case "reprendre":
			p.Pause(false)
		case "jour":
			p.JourSuivant()
		case "reinit":
			p.Reinitialiser()
		case "vitesse":
			if c.DelaiMs < 0 || c.DelaiMs > 2000 {
				http.Error(w, "delai_ms hors bornes [0, 2000]", http.StatusBadRequest)
				return
			}
			p.Delai(time.Duration(c.DelaiMs) * time.Millisecond)
		default:
			http.Error(w, "action inconnue", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// Servir démarre le serveur et l'arrête proprement à l'annulation du contexte
func Servir(ctx context.Context, adresse string, p *sma.Pilote) error {
	srv := &http.Server{Addr: adresse, Handler: Routeur(p), ReadHeaderTimeout: 5 * time.Second}
	// Préparer l'arrêt du serveur en arrière plan
	go func() {
		<-ctx.Done()
		arret, annuler := context.WithTimeout(context.Background(), 2*time.Second)
		defer annuler()
		_ = srv.Shutdown(arret)
	}()
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
