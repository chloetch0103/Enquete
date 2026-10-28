package sma

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// --- Client LLM local (API Ollama /api/chat) ---
//
// Le modèle tourne sur la machine

// ClientLLM interroge un serveur Ollama
type ClientLLM struct {
	URL    string
	Modele string
	http   *http.Client
	sem    chan struct{} // limite le nombre de requêtes simultanées
}

func NouveauClientLLM(url, modele string, timeout time.Duration, parallele int) *ClientLLM {
	if parallele < 1 {
		parallele = 1
	}
	return &ClientLLM{
		URL: strings.TrimRight(url, "/"), Modele: modele,
		http: &http.Client{Timeout: timeout},
		sem:  make(chan struct{}, parallele),
	}
}

type messageChat struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type requeteChat struct {
	Model    string         `json:"model"`
	Messages []messageChat  `json:"messages"`
	Stream   bool           `json:"stream"`
	Format   string         `json:"format,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

type reponseChat struct {
	Message messageChat `json:"message"`
}

// Chat envoie un prompt système + utilisateur et renvoie le texte produit.
func (c *ClientLLM) Chat(ctx context.Context, systeme, utilisateur string) (string, error) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	corps, err := json.Marshal(requeteChat{
		Model: c.Modele,
		Messages: []messageChat{
			{Role: "system", Content: systeme},
			{Role: "user", Content: utilisateur},
		},
		Format:  "json",                             // sortie contrainte en JSON
		Options: map[string]any{"temperature": 0.0}, // décisions aussi stables que possible
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/api/chat", bytes.NewReader(corps))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// Cas où Ollama répond bien mais pour signaler une erreur
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("ollama: statut %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var r reponseChat
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("ollama: réponse illisible: %w", err)
	}
	return r.Message.Content, nil
}

// --- Stratégie de négociation déléguée au LLM ---

// StrategieLLM délègue la décision "accepter / refuser" au LLM et garde la
// stratégie à règles comme garde-fou : en cas d'erreur, de délai dépassé ou de
// réponse invalide, l'agent se replie sur les règles (l'agent reste fonctionnel
// même si le modèle tombe).
type StrategieLLM struct {
	Strategie // stratégie de repli
	client    *ClientLLM
	journal   *JournalLLM
}

// FabriqueLLM renvoie une fabrique de stratégies LLM pour NouveauMonde.
func FabriqueLLM(client *ClientLLM, journal *JournalLLM) FabriqueStrategie {
	return func(t TypeAgent) Strategie {
		return &StrategieLLM{Strategie: NouvelleStrategie(t), client: client, journal: journal}
	}
}

var personnalites = map[TypeAgent]string{
	Altruiste: "Tu es altruiste : tu privilégies la survie du groupe et la coopération, même à ton désavantage.",
	Neutre:    "Tu es équitable : tu coopères volontiers si le partage est juste, tu refuses d'être exploité,",
	Egoiste:   "Tu es égoïste: tu maximises ta propre énergie et ta survie, sans considération pour les autres.",
}

func promptSysteme(t TypeAgent) string {
	return personnalites[t] + `
Tu es un agent dans une simulation multi-agents. Chaque soir tu dois avoir assez d'énergie pour survivre.
Les pommes se mangent seul. Une vache ne peut être chassée qu'à deux : au moment du partage, chacun
réclame une part. Si la somme des parts dépasse 100 %, la vache est perdue pour les deux.
Réponds UNIQUEMENT avec un objet JSON de la forme {"decision": "accepte" | "refuse", "raison": "<une phrase courte>"}.`
}

func promptNegociation(c ContexteNegociation) string {
	return fmt.Sprintf(`Tick %d. Ton énergie : %d. Coût de survie ce soir : %d. Ticks avant la fin de la journée : %d.
Pommes visibles autour de toi : %d.
L'agent #%d te propose de chasser ensemble une vache qui vaut %d d'énergie. Il réclame %.0f %% de la vache.
Si tu acceptes, tu réclameras %.0f %% (total %.0f %%).
Acceptes-tu ?`,
		c.Tick, c.MonEnergie, c.CoutJournalier, c.TicksAvantFinJour, c.PommesVisibles,
		c.Proposant, c.EnergieVache, c.DemandeProposant*100,
		c.MaRevendication*100, (c.DemandeProposant+c.MaRevendication)*100)
}

// analyserDecision extrait {"decision": .., "raison": ...} de la réponse du modèle
func analyserDecision(texte string) (Decision, error) {
	debut, fin := strings.Index(texte, "{"), strings.LastIndex(texte, "}")
	if debut < 0 || fin <= debut {
		return Decision{}, errors.New("pas d'objet JSON dans la réponse")
	}
	var r struct {
		Decision string `json:"decision"`
		Raison   string `json:"raison"`
	}
	if err := json.Unmarshal([]byte(texte[debut:fin+1]), &r); err != nil {
		return Decision{}, fmt.Errorf("JSON invalide: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(r.Decision)) {
	case "accepte", "accepter", "accept", "oui", "yes":
		return Decision{Accepte: true, Raison: r.Raison}, nil
	case "refuse", "refuser", "refus", "non", "no":
		return Decision{Accepte: false, Raison: r.Raison}, nil
	}
	return Decision{}, fmt.Errorf("décision inconnue %q", r.Decision)
}

// Repondre : appelé en parallèle par plusieurs agents (le client est sûr).
func (s *StrategieLLM) Repondre(c ContexteNegociation) Decision {
	ctx, annuler := context.WithTimeout(context.Background(), s.client.http.Timeout)
	defer annuler()
	debut := time.Now()
	texte, err := s.client.Chat(ctx, promptSysteme(s.Type()), promptNegociation(c))
	var d Decision
	if err == nil {
		d, err = analyserDecision(texte)
	}
	source := "llm"
	if err != nil {
		d = s.Strategie.Repondre(c)
		source = "repli"
	}
	s.journal.Ajouter(EntreeLLM{
		Tick: c.Tick, Agent: c.Moi.String(), Proposant: c.Proposant, Demande: c.DemandeProposant,
		Accepte: d.Accepte, Raison: d.Raison, Source: source, Erreur: erreurTexte(err),
		DureeMs: time.Since(debut).Milliseconds(),
	})
	d.Raison = "[" + source + "] " + d.Raison
	return d
}

func erreurTexte(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// --- Journal des décisions ---

// EntreeLLM : une décision sérialisée (sera stockée dans le json line)
type EntreeLLM struct {
	Tick      int     `json:"tick"`
	Agent     string  `json:"agent"`
	Proposant int     `json:"proposant"`
	Demande   float64 `json:"demande"`
	Accepte   bool    `json:"accepte"`
	Raison    string  `json:"raison"`
	Source    string  `json:"source"` // "llm" ou "repli"
	Erreur    string  `json:"erreur,omitempty"`
	DureeMs   int64   `json:"duree_ms"`
}

// JournalLLM est sûr en accès concurrent
type JournalLLM struct {
	mu            sync.Mutex
	enc           *json.Encoder
	appels, repli int
	dureeTotale   int64
}

// NouveauJournalLLM : w peut être nil (pas d'écriture, seulement des compteurs)
func NouveauJournalLLM(w io.Writer) *JournalLLM {
	j := &JournalLLM{}
	if w != nil {
		j.enc = json.NewEncoder(w)
	}
	return j
}

func (j *JournalLLM) Ajouter(e EntreeLLM) {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.appels++
	j.dureeTotale += e.DureeMs
	if e.Source == "repli" {
		j.repli++
	}
	if j.enc != nil {
		_ = j.enc.Encode(e)
	}
}

// Resume : nombre d'appels, taux de repli et latence moyenne.
func (j *JournalLLM) Resume() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.appels == 0 {
		return "aucun appel"
	}
	return fmt.Sprintf("%d décisions, %d replis sur règles (%.0f %%), latence moyenne %d ms",
		j.appels, j.repli, 100*float64(j.repli)/float64(j.appels), j.dureeTotale/int64(j.appels))
}
