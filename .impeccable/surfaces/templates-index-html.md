---
version: 1
slug: "templates-index-html"
primary_target: "templates/index.html"
related_targets: []
---

Scope: application web mono-entreprise, toutes les pages (factures, clients, import, assistant). Mode: Operate.

Audience: gérant de PME, seul, quelques factures par mois. Tâche: suivre les encaissements, créer et importer des factures, poser une question en français.

Contenu: factures réelles (numéro, client, dates, statut, montant), lignes, clients, résumé des totaux. Contraintes: français uniquement, aucun login, données locales, assistant en lecture seule.

Direction: Journal à colonnes (candidat retenu, 3e de la liste, tirage concept-seed). Moment mémorable: la colonne de solde reste visible pendant que les lignes défilent et se clôt par un double trait.

RAISE — rail de statut: le statut vit dans une colonne encrée dédiée, avec formes de trait (plein / pointillé / barré), apport du rail spectrographique.
RAISE — onglet rubrique: les lignes impayées portent un onglet rouge, apport des modules denses.
RAISE — marge d'appareil: l'assistant occupe une marge réglée à côté du journal, apport de la référence à marge.
RAISE — numéro d'entrée: le numéro de facture est l'identifiant titre, apport du catalogue Saville.
RAISE — cellule active cerclée: la ligne active est cerclée à l'encre, apport du catalogue de salon.
RAISE — langue des encres: couleurs de statut fixes, l'assistant se superpose sans les modifier, apport de la carte d'orientation.

Résolus: aucune identité de marque imposée. Non résolus: impression papier, rôles multiples.

## Direction contract

THESIS: Un journal de comptabilité à colonnes, pas un tableau de bord de cartes. La facture est une écriture dans un registre ; l'argent se lit dans une colonne de solde, pas dans des vignettes. Le chat n'est pas une fenêtre flottante mais un appareil de marge.

OWN-WORLD: Fond bone.chaud, encre navy pour le texte, rubric rouge pour l'impayé, vert encre pour l'acquitté, filets 1px gris-encre. Grotesque de travail compact, chiffres tabulaires, titre en numéro d'entrée. Signature: colonne de solde collante avec double trait de clôture et tampons d'encre minuscules.

STORY: Le gérant voit d'abord ce qui reste dû, repère la ligne par son numéro, l'ouvre, puis demande à l'assistant « qu'est-ce qui arrive en décembre ? » sans quitter l'écran.

FIRST VIEWPORT: Rail latéral étroit (Factures, Clients, Importer, Assistant). En-tête: titre « Journal des factures » et action « Nouvelle facture ». Bandeau réglé de totaux en une ligne, sans cartes. Table dense:Numéro / Client / Émission / Échéance / Statut / Solde, colonne Solde collante à droite. Ligne impayée avec onglet rubric. Marge droite réservée à l'assistant, repliable.

FORM: Journal à colonnes, 3e de la liste ordonnée (7 candidats). seed: eaa48d849e9b116b.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
