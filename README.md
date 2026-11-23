# Journal des factures

Application web locale pour gérer la facturation d'une PME : clients, factures, import et export Excel, statistiques et assistant IA. Elle fonctionne sans serveur de base de données : tout est stocké dans un fichier SQLite.

## Fonctionnalités

- **Factures** : création avec plusieurs lignes (description, quantité, prix unitaire), numérotation automatique `F-2026-0001`, statut *Due* / *Payée*, encaissement et suppression.
- **Clients** : répertoire simple, réutilisé lors de la création des factures.
- **Export XLSX** : téléchargement de toutes les factures dans un classeur `factures.xlsx` (feuille « Factures », une ligne par facture, ligne de total).
- **Import XLSX** : un fichier Excel est lu, puis un modèle OpenRouter convertit ses lignes en clients, factures et lignes de factures.
- **Statistiques** : totaux, production mensuelle sur 12 mois, clients les plus facturés et ancienneté des impayés.
- **Assistant** : questions en langage naturel sur les factures (impayés, échéances, montants). Il est **en lecture seule** : il ne modifie jamais les données.

L'interface est en français et suit une direction visuelle de « journal de comptabilité à colonnes » : rail de navigation sombre, table réglée au filet, solde toujours visible, totaux soulignés d'un double trait.

## Prérequis

- **Go 1.22 ou supérieur** (les routes HTTP utilisent les motifs avec méthode et paramètres de chemin).
- Aucune installation de SQLite : le pilote `modernc.org/sqlite` est en Go pur, sans CGO.
- Une clé **OpenRouter** uniquement si vous utilisez l'import XLSX ou l'assistant.

## Démarrage

```bash
cd /home/theneovimmer/Documents/excel
go run .
```

Ouvrez ensuite <http://localhost:8080>. Le fichier `invoices.db` est créé automatiquement au premier lancement, avec les tables et la vue nécessaires.

Pour compiler un binaire autonome :

```bash
go build -o journal-factures .
./journal-factures
```

Les templates HTML et la feuille de style sont embarqués dans le binaire : aucun dossier annexe n'est nécessaire à l'exécution.

## Configuration

La configuration se fait par variables d'environnement. Aucune clé n'est écrite dans le code source. Un modèle est fourni dans `.env.example` :

```bash
cp .env.example .env
# éditez .env pour remplacer sk-or-v1-votre-cle-ici par votre clé
set -a && source .env && set +a
go run .
```

Le fichier `.env` est ignoré par Git et ne doit jamais être versionné. L'application ne lit pas `.env` automatiquement : la commande `source` ci-dessus charge les variables dans le shell avant le lancement.

| Variable | Défaut | Rôle |
|---|---|---|
| `PORT` | `8080` | Port d'écoute du serveur web |
| `DB_PATH` | `invoices.db` | Chemin du fichier SQLite |
| `OPENROUTER_API_KEY` | *(vide)* | Clé API OpenRouter, requise pour l'import et l'assistant |
| `OPENROUTER_MODEL` | `google/gemma-4-26b-a4b-it:free` | Modèle essayé en premier |

Exemple :

```bash
export OPENROUTER_API_KEY='votre-cle-openrouter'
export PORT=8080
export DB_PATH='/home/theneovimmer/Documents/excel/invoices.db'
go run .
```

Si le modèle demandé est momentanément saturé (erreur 429, courante sur les modèles gratuits), l'application bascule automatiquement vers `google/gemma-4-31b-it:free`, puis vers `nvidia/nemotron-3.5-lightning:free`.

## Comment fonctionne l'import XLSX

1. Le fichier est envoyé depuis la page **Importer XLSX** (10 Mo maximum).
2. Le premier onglet est lu avec `excelize` ; les 100 premières lignes sont transformées en texte tabulé.
3. Ce texte est envoyé à OpenRouter avec une consigne qui demande un JSON de clients, factures et lignes.
4. Le JSON est validé puis inscrit en base dans une seule transaction : les clients sont dédupliqués par nom, les numéros de facture existants sont conservés, les autres sont générés.

Si le modèle renvoie une réponse invalide, aucun changement n'est appliqué. Les montants saisis avec une virgule ou un point décimal sont convertis en centimes, pour éviter les erreurs d'arrondi.

## Comment fonctionne l'assistant

La page envoie la question à `POST /api/chat`. Le serveur construit un instantané compact des factures (totaux, nombre d'impayés, échéances et montants), l'ajoute à la question et appelle OpenRouter. La réponse est courte, en français, et affichée dans la marge de l'interface. Aucune requête d'écriture n'est exposée au chatbot.

## Statistiques et modèle de données

La page **Statistiques** lit une vue SQL unique, `invoice_facts`, qui contient une ligne par facture avec son client, ses dates, son statut et son total calculé. C'est la couche de mesure commune : tous les indicateurs en sont dérivés.

| Objet | Contenu |
|---|---|
| `clients` | Nom, email, adresse |
| `invoices` | Numéro, client, dates, statut, notes |
| `invoice_items` | Lignes de facture : description, quantité, prix unitaire en centimes |
| `invoice_facts` | Vue d'agrégation utilisée par les statistiques et l'assistant |

## Routes principales

| Route | Action |
|---|---|
| `GET /` | Journal des factures |
| `GET /invoices/new` | Formulaire de création |
| `POST /invoices` | Création d'une facture |
| `GET /invoices/{id}` | Détail d'une facture |
| `POST /invoices/{id}/pay` | Marquer comme payée |
| `POST /invoices/{id}/delete` | Supprimer une facture |
| `GET /invoices/export.xlsx` | Télécharger l'export Excel |
| `GET /clients`, `POST /clients` | Répertoire des clients |
| `GET /import`, `POST /import` | Import Excel assisté par IA |
| `GET /stats` | Statistiques |
| `POST /api/chat` | Question à l'assistant |

## Structure du projet

```text
main.go            serveur, routes, templates embarqués
db.go              schéma SQLite et numérotation
handlers.go        factures et clients
import.go          lecture XLSX et appel OpenRouter pour l'import
export.go          génération du classeur XLSX
chat.go            instantané des factures et endpoint de chat
stats.go           indicateurs et agrégations
templates/         interfaces HTML
static/style.css   système visuel
PRODUCT.md         contexte produit
```

## Limites à connaître

- Une seule entreprise, aucun compte utilisateur ni gestion des rôles.
- La devise affichée est l'euro ; elle est définie par la constante `currency` dans `main.go`.
- L'import ne lit que le premier onglet et les 100 premières lignes.
- Le contenu des fichiers importés et un résumé des factures sont envoyés à OpenRouter : évitez les données confidentielles si ce n'est pas souhaité.
- Une seule connexion SQLite est utilisée à la fois, ce qui convient à un usage de PME mais pas à une forte concurrence.

## Dépannage

- **`OPENROUTER_API_KEY non définie`** : exportez la variable avant de lancer le serveur, puis redémarrez-le.
- **`temporarily rate-limited upstream`** : les modèles gratuits sont parfois saturés ; l'application essaie les suivants automatiquement. Vous pouvez aussi choisir un autre modèle avec `OPENROUTER_MODEL`.
- **`Fichier Excel illisible`** : seuls les `.xlsx` sont acceptés, et le fichier doit faire moins de 10 Mo.
- **`bind: address already in use`** : le port est occupé ; lancez le serveur avec `PORT=8081 go run .`
