# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Le gérant d'une PME, seul utilisateur, quelques factures par mois. Il saisit ses clients et factures, importe des fichiers Excel, et pose des questions en langage naturel sur l'état de sa facturation. Pas de comptable ni d'équipe dans le périmètre actuel.

## Product Purpose

Gérer les factures d'une seule entreprise : créer des clients, créer des factures avec lignes, suivre le statut payé/impayé, importer et exporter en XLSX, et obtenir des réponses sur les données par chat. Le succès est une facturation tenue à jour avec un minimum d'effort de saisie.

## Positioning

Outil local mono-entreprise qui boucle le cycle du fichier Excel : export XLSX, réimport XLSX assisté par un modèle OpenRouter, et interrogation en langage naturel des factures — sans base serveur partagée ni comptes utilisateurs.

## Operating Context

Application web servie en local (Go, templatesHTML embarqués, SQLite en fichier) et ouverte dans un navigateur. Les données sont des clients (nom, email, adresse) et des factures (numéro, client, dates, statut, notes, lignes avec quantité et prix unitaire). Un import XLSX envoie le contenu du fichier à OpenRouter ; le chat envoie un résumé des factures au même fournisseur. Langue de travail : français.

## Capabilities and Constraints

- Une seule entreprise, aucun login, aucune isolation multi-utilisateur.
- Interface en français uniquement.
- Montants stockés en centiers ; devise affichée en euros.
- Import XLSX : premier onglet, 100 premières lignes, 10 Mo maximum, mapping assuré par OpenRouter avec modèles gratuits et repli automatique.
- Export XLSX : une ligne par facture, feuille « Factures ».
- Chatbot en lecture seule : répond aux questions sur les factures, ne modifie aucune donnée.
- Clé OpenRouter fournie par variable d'environnement, jamais codée en dur.

## Brand Commitments

Aucune identité visuelle ni nom de marque imposés par l'utilisateur.

## Evidence on Hand

Application existante dans `/home/theneovimmer/Documents/excel` : `main.go`, `db.go`, `handlers.go`, `import.go`, `export.go`, `templates/`, `static/style.css`. Routes : factures, clients, import XLSX, export XLSX. Aucun logo, palette ou directive de marque fourni ; ne pas en inventer.

## Product Principles

- La donnée reste locale : SQLite en fichier, aucune infrastructure partagée.
- La saisie est minimale : Excel entre et sort, le modèle fait le travail de correspondance.
- Une seule source de vérité : la facture, avec son client et ses lignes.
- Le chat éclaire, il n'agit pas : aucune mutation de données depuis le chatbot.
- Le français partout : libellés, messages, réponses du chat.
