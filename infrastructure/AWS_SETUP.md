# AWS Setup für Deploy-Workflows

Anleitung für alles, was **einmalig** in einem AWS-Account passieren muss, bevor
die GitHub-Actions-Workflows (`Deploy Dev`, `Deploy Staging`, `Deploy Production`)
durchlaufen können.

## Wann brauchst du das?

| Szenario | Was tun? |
|---|---|
| Das Team teilt sich **einen** AWS-Account, jemand hat schon einmal deployed | **Nichts.** Bootstrap + OIDC-Provider gelten accountweit. Du brauchst nur Code-Zugriff. |
| Du benutzt deinen **eigenen** AWS-Account für lokales Testen | Schritte 1–3 einmalig durchziehen. |
| Du willst eine **neue Region** nutzen (statt `eu-central-1`) | Nur Schritt 2 (CDK Bootstrap) für die neue Region. OIDC-Provider ist regionsunabhängig. |
| Repo gets forked / umbenannt | Schritt 1 erneut (oder Stack-Parameter ändern), da die IAM-Rolle den Repo-Pfad als Trust-Bedingung hat. |

Stand: Account `819926065066` ist bereits gebootstrappt für `eu-central-1`.

---

## Schritt 1 — IAM-Rolle für GitHub Actions (OIDC)

Erzeugt einen OIDC Identity Provider + eine IAM-Rolle, die GitHub Actions
annehmen kann — nur aus den Branches `aws-test` und `dev/*` dieses Repos.

1. AWS Console → Region oben rechts auf **eu-central-1** (Frankfurt) stellen.
2. **CloudFormation** öffnen → **Create stack** → **With new resources (standard)**.
3. **Upload a template file** → [github-oidc-bootstrap.yml](github-oidc-bootstrap.yml) hochladen.
4. **Stack name**: `msa2-github-oidc`. Parameter so lassen.
5. Bei "Capabilities" das Häkchen *"I acknowledge…IAM resources with custom names"* setzen.
6. **Submit** → ca. 30 Sek warten bis `CREATE_COMPLETE`.
7. Tab **Outputs** öffnen — drei Werte für die GitHub-Secrets (siehe Schritt 3).

> Die Rolle bekommt `AdministratorAccess`. Für eine PoC ok, für Produktion
> nicht. Wer das tightenen will: Managed Policy im Template tauschen.

---

## Schritt 2 — CDK Bootstrap

CDK braucht im Ziel-Account einen S3-Bucket + IAM-Rollen für Asset-Upload und
Deployment. Ohne den scheitert jeder `cdk deploy` mit *"No bootstrap stack found"*.

**Einfachster Weg (kein AWS CLI lokal nötig):** AWS CloudShell

1. AWS Console → Region auf **eu-central-1**.
2. Oben in der Header-Leiste auf das **CloudShell-Icon** (`>_`).
3. Befehl (Account-ID ggf. anpassen):

   ```bash
   npx --yes aws-cdk@2 bootstrap aws://<DEINE-ACCOUNT-ID>/eu-central-1
   ```

4. Warten auf `✅ Environment ... bootstrapped`. Dauert ~1 Min.

**Alternativ lokal:** AWS CLI installieren, `aws configure` mit eigenem Access
Key, dann `npx cdk bootstrap aws://<account>/eu-central-1` aus `infrastructure/`.

Das Ergebnis ist ein CloudFormation-Stack namens **`CDKToolkit`**. Erneutes
Ausführen ist idempotent (sagt nur "no changes").

---

## Schritt 3 — GitHub Secrets

In GitHub: **Repo Settings → Secrets and variables → Actions** → **New repository secret**.

| Secret-Name | Wert | Quelle |
|---|---|---|
| `AWS_REGION` | `eu-central-1` | fest |
| `AWS_ROLE_ARN_DEV` | `arn:aws:iam::<account>:role/msa2-github-deploy-dev` | Output `RoleArn` aus Stack `msa2-github-oidc` |
| `ECR_REGISTRY` | `<account>.dkr.ecr.eu-central-1.amazonaws.com` | Output `EcrRegistry` aus Stack `msa2-github-oidc` |

Für `staging` und `production` analog: Secrets `AWS_ROLE_ARN_STAGING` /
`AWS_ROLE_ARN_PRODUCTION` (eigene Rollen im OIDC-Template ergänzen).

> **Hinweis zu GitHub Environments:** Auf privaten Org-Repos im Free-Plan sind
> *Environments* (Settings → Environments) nicht verfügbar — daher referenziert
> der Dev-Workflow keines. Falls eure Org später ein Upgrade hat, kann man
> Environments für Approvals/Branch-Protections vor Deploys nutzen.

---

## Deploy testen

Auf einen der trigger-Branches pushen (`aws-test` oder `dev/<irgendwas>`):

```bash
git push origin aws-test
```

GitHub → Actions → Workflow **"2 · Deploy → AWS (dev)"** beobachten.
Erster Lauf: **15–25 Min** (Aurora, ElastiCache, ECS Fargate werden erzeugt).
Folge-Läufe: 3–8 Min (nur Image-Push + Service-Update).

URL des deployten Frontends: in der AWS Console unter **EC2 → Load Balancers**
oder im Output des `Msa2-Dev-Core` CloudFormation-Stacks.

---

## Aufräumen (wichtig: Kosten!)

Aurora + ElastiCache + ALB laufen 24/7 und kosten zusammen **~7–12 €/Tag**.
Wenn nicht aktiv getestet wird:

```bash
# in CloudShell oder lokal mit Credentials
cd infrastructure
npx cdk destroy --all -c environment=dev
```

Was bleibt nach `destroy --all`:
- `CDKToolkit`-Stack (Bootstrap) — kann bleiben, kostet praktisch nichts.
- `msa2-github-oidc`-Stack — kann bleiben, kostet nichts.
- ECR-Repos mit Images — manuell löschen falls gewünscht (Console → ECR).

---

## Troubleshooting

| Symptom | Ursache |
|---|---|
| Workflow-Fehler `Could not assume role` / `AccessDenied` auf `sts:AssumeRoleWithWebIdentity` | Branch passt nicht zum Trust der Rolle (`aws-test`, `dev/*`). Stack-Parameter prüfen. |
| `No bootstrap stack found` während Deploy | Schritt 2 nicht gelaufen für den Account/Region. |
| `Stack ... is in ROLLBACK_COMPLETE state and can not be updated` | Im Console-CloudFormation den Stack löschen, dann Workflow neu starten. |
| ECR-Push-Fehler `repository does not exist` | Sollte nicht passieren — die Composite Action erzeugt fehlende Repos. Falls doch: Rolle hat keine `ecr:CreateRepository`-Permission (Trust ggf. zu eng). |
