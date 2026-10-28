"""Trace docs/resultats.png à partir des CSV produits en mode -headless.

Étape 1 : générer un CSV par graine (depuis la racine du projet)
    foreach ($s in 1..10) { go run . -headless -jours 40 -seed $s -csv "runs/s$s.csv" }

Étape 2 : tracer
    python scripts/graphiques.py runs docs/resultats.png

Dépendance : pip install matplotlib
"""
import csv
import glob
import os
import statistics as st
import sys

import matplotlib

matplotlib.use("Agg")  # pas de fenêtre : on écrit directement l'image
import matplotlib.pyplot as plt

dossier = sys.argv[1] if len(sys.argv) > 1 else "runs"
sortie = sys.argv[2] if len(sys.argv) > 2 else "docs/resultats.png"

# Une simulation = une liste de lignes (une par jour), chaque ligne étant un dict colonne -> valeur.
fichiers = sorted(glob.glob(os.path.join(dossier, "*.csv")))
if not fichiers:
    sys.exit(f"Aucun CSV trouvé dans {dossier}")
runs = []
for f in fichiers:
    with open(f, newline="", encoding="utf-8") as fh:
        runs.append(list(csv.DictReader(fh)))

# Une simulation peut s'arrêter plus tôt si tout le monde meurt :
# on ne moyenne que sur les jours communs à toutes les simulations.
L = min(len(r) for r in runs)

types = {
    "altruiste": ("#2fa866", "altruistes"),
    "neutre": ("#3b7be0", "neutres"),
    "egoiste": ("#f08a24", "égoïstes"),
}

fig, (ax, ax2) = plt.subplots(1, 2, figsize=(11, 4), dpi=150)

# --- Graphique de gauche : population par type ---
for t, (couleur, libelle) in types.items():
    col = f"population_{t}"
    # chaque simulation en trait fin et transparent
    for r in runs:
        ax.plot([int(ligne[col]) for ligne in r], color=couleur, alpha=0.12, lw=1)
    # la moyenne sur toutes les graines en trait épais
    moyenne = [st.mean(int(r[j][col]) for r in runs) for j in range(L)]
    ax.plot(moyenne, color=couleur, lw=2.4, label=libelle)
ax.set_title(f"Population par type ({len(runs)} graines, moyenne en trait épais)", fontsize=10)
ax.set_xlabel("jour")
ax.legend(frameon=False)

# --- Graphique de droite : coopération vs conflit ---
# On commence au jour 1 : la ligne du jour 0 ne contient que la population initiale.
for col, libelle, couleur in [
    ("vaches_partagees", "vaches partagées", "#2fa866"),
    ("vaches_gachees", "vaches gâchées (conflit)", "#d64545"),
]:
    moyenne = [st.mean(int(r[j][col]) for r in runs) for j in range(1, L)]
    ax2.plot(range(1, L), moyenne, label=libelle, color=couleur, lw=2)
ax2.set_title("Coopération vs conflit (moyenne par jour)", fontsize=10)
ax2.set_xlabel("jour")
ax2.legend(frameon=False)

for a in (ax, ax2):
    a.spines[["top", "right"]].set_visible(False)
    a.grid(alpha=0.25)

fig.tight_layout()
os.makedirs(os.path.dirname(sortie) or ".", exist_ok=True)
fig.savefig(sortie)
print(f"{len(runs)} simulations tracées --> {sortie}")