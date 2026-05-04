# BatAudit Fork — Plano de Manutenção

> Fork de [`joaovrmoraes/bataudit`](https://github.com/joaovrmoraes/bataudit) hospedado em [`anulamulta/bataudit`](https://github.com/anulamulta/bataudit).
>
> **Caminho G (monorepo integrado) — atualizado 2026-05-04.** Toda a infraestrutura (EC2 + ALB rules + DNS + Backup + SSM) e o SDK Node consumido pelos services V2 vivem agora no monorepo `anulamulta-platform`. Este repositório (`anulamulta/bataudit`) tem responsabilidades reduzidas:
>
> 1. **Build pipeline** — workflow `release-images.yml` que builda 3 imagens Docker (writer/reader/worker) e pusha em ECR conta `240806822693`.
> 2. **Sync com upstream** — manter o fork atualizado com `joaovrmoraes/bataudit`; receber mudanças de SDK/código Go e propagar para o monorepo via PR.
> 3. **Patches específicos** do AnulaMulta — ex. `REDIS_DB` env var support.
>
> **Doc canônico do consumo (IaC + SDK):** `anulamulta-platform/docs/ETAPA_2_IMPLEMENTATION_PLAN.md` §"Track BatAudit".

---

## 0. Contexto

### O que mora aqui

| Componente                                       | Status                                                                                |
| ------------------------------------------------ | ------------------------------------------------------------------------------------- |
| Código Go (`cmd/api/{writer,reader,worker}/`)    | ✅ Mantido — sync com upstream                                                        |
| Internal packages (`internal/`)                  | ✅ Mantido                                                                             |
| SDKs (`sdks/node/`, `sdks/browser/`)             | ✅ Mantido como **fonte canônica**; vendoreado periodicamente em `anulamulta-platform/packages/core/src/bataudit/` |
| Frontend dashboard React (`frontend/`)           | ✅ Mantido — buildado dentro da imagem `bataudit-reader`                              |
| Dockerfiles (`cmd/api/*/Dockerfile`)             | ✅ Mantidos — usados pelo workflow                                                    |
| `docker-compose.yml`                             | ✅ Mantido — usado pelo EC2 BatAudit (ANU-107) via clone do fork no user_data         |
| **Infrastructure Terraform**                     | ❌ **Removida** — IaC está em `anulamulta-platform/infrastructure/` (ANU-107 a ANU-113) |
| **Workflow `release-sdk.yml`**                   | ❌ **Não criada** — SDK não publica em npm; é vendoreado direto                       |
| **`docs/IMPLEMENTATION_PLAN.md`**                | ✅ Este arquivo, com escopo reduzido                                                   |

### Decisões consolidadas

| #   | Decisão                                                              | Resposta                                                                              |
| --- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| 1   | Hospedagem do BatAudit                                               | EC2 t4g.small + docker-compose dentro da VPC `anulamulta-staging` (Caminho G)         |
| 2   | SDK distribution                                                      | **Vendor** em `anulamulta-platform/packages/core/src/bataudit/` (workspace local; sem npm publish) |
| 3   | Repositório do fork                                                   | `github.com/anulamulta/bataudit` — sync periódico com upstream                        |
| 4   | ECR repos                                                             | `anulamulta/bataudit-{writer,worker,reader}` na conta `240806822693`, gerenciados pelo tfstate `anulamulta-platform/infrastructure/environments/staging/` |
| 5   | OIDC role para push                                                  | `github-actions-deploy-staging` (existente) com trust policy estendida cobrindo `repo:anulamulta/bataudit:*` (ANU-115) |
| 6   | Sync upstream                                                         | Manual periódico (`git fetch upstream && git merge upstream/main`); sem automação no fork |

### Custo deste repo

**Zero AWS.** Todos os custos AWS (~$18/mês — EC2 + EBS + Backup) ficam no projeto `anulamulta-platform`.

---

## 1. Issues no Linear (no projeto **Backend Anula Multa V2**)

Mudanças de escopo: as 3 issues que afetam este repo viram **ANU-115, ANU-116, ANU-117** (não BAT-* mais). Razão: Caminho G unificou planejamento sob projeto AnulaMulta.

### ANU-115 — OIDC trust policy estendida cobrindo `repo:anulamulta/bataudit`

- **Track:** Infra (no monorepo `anulamulta-platform`)
- **Branch:** `caiolcolaco/anu-115-oidc-trust-policy-estendida-bataudit`
- **Dependência:** ANU-61 (✅ Etapa 1)
- **Refs:** `anulamulta-platform/docs/ETAPA_2_IMPLEMENTATION_PLAN.md` §3 ANU-115

**Objetivo:** estender trust policy do role IAM `github-actions-deploy-staging` (criado na Etapa 1) para aceitar OIDC tokens do repo `anulamulta/bataudit`, permitindo que o workflow `release-images.yml` (ANU-116) faça push para os 3 ECR repos `anulamulta/bataudit-*`.

**Atividades:**

* Editar `infrastructure/modules/github-oidc/main.tf` no `anulamulta-platform` adicionando `repo:anulamulta/bataudit:ref:refs/heads/main` e `repo:anulamulta/bataudit:ref:refs/tags/v*` na condition `StringLike` da trust policy.
* `terraform apply` manual.
* Validar: `aws iam get-role --role-name github-actions-deploy-staging --query 'Role.AssumeRolePolicyDocument.Statement[].Condition.StringLike."token.actions.githubusercontent.com:sub"'` retorna 3 entries (existente + 2 novas).

**Critérios de Aceitação:**

* Trust policy lista 3 patterns sub.
* Smoke test: workflow do fork em branch consegue assumir o role.

---

### ANU-116 — Workflow `release-images.yml` no fork (build+push 3 imagens ECR)

- **Track:** Bootstrap (este repo, `anulamulta/bataudit`)
- **Branch:** `caiolcolaco/anu-116-workflow-release-images-yml-fork-bataudit`
- **Dependência:** ANU-115
- **Refs:** este doc §2

**Objetivo:** workflow GitHub Actions no repo `anulamulta/bataudit` que dispara em push de tag `v*` e publica 3 imagens ARM64 em ECR conta `240806822693`.

**Atividades:**

* Criar `.github/workflows/release-images.yml` com 2 jobs:
  * `build-and-push` (matrix services: writer, reader, worker)
    * OIDC assume `arn:aws:iam::240806822693:role/github-actions-deploy-staging`
    * `aws-actions/amazon-ecr-login`
    * Buildx + Dockerfile multi-stage Go
    * `platforms: linux/arm64` (alvo t4g.small ARM Graviton)
    * Tags: `v0.1.0` + `latest` + SHORT_SHA
    * Cache via `type=gha,mode=max`
  * `scan` (depende do build)
    * `aws ecr wait image-scan-complete`
    * Falha se findings CRITICAL > 0
* Smoke test em branch antes do primeiro release oficial
* Push `v0.1.0` na main do fork dispara primeiro release

```yaml
name: Release Images
on:
  push: { tags: ['v*'] }
permissions:
  id-token: write
  contents: read
jobs:
  build-and-push:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        service: [writer, reader, worker]
    steps:
      - uses: actions/checkout@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::240806822693:role/github-actions-deploy-staging
          aws-region: us-east-1
      - uses: aws-actions/amazon-ecr-login@v2
      - uses: docker/setup-buildx-action@v3
      - id: version
        run: echo "VERSION=${GITHUB_REF#refs/tags/}" >> $GITHUB_OUTPUT
      - uses: docker/build-push-action@v5
        with:
          context: .
          file: cmd/api/${{ matrix.service }}/Dockerfile
          platforms: linux/arm64
          push: true
          tags: |
            240806822693.dkr.ecr.us-east-1.amazonaws.com/anulamulta/bataudit-${{ matrix.service }}:${{ steps.version.outputs.VERSION }}
            240806822693.dkr.ecr.us-east-1.amazonaws.com/anulamulta/bataudit-${{ matrix.service }}:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
  scan:
    needs: build-and-push
    runs-on: ubuntu-latest
    strategy:
      matrix: { service: [writer, reader, worker] }
    steps:
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::240806822693:role/github-actions-deploy-staging
          aws-region: us-east-1
      - run: |
          aws ecr wait image-scan-complete \
            --repository-name anulamulta/bataudit-${{ matrix.service }} \
            --image-id imageTag=${GITHUB_REF#refs/tags/}
      - run: |
          findings=$(aws ecr describe-image-scan-findings \
            --repository-name anulamulta/bataudit-${{ matrix.service }} \
            --image-id imageTag=${GITHUB_REF#refs/tags/} \
            --query 'imageScanFindings.findingSeverityCounts')
          critical=$(echo "$findings" | jq -r '.CRITICAL // 0')
          if [ "$critical" -gt 0 ]; then
            echo "::error::Found $critical CRITICAL vulnerabilities"
            exit 1
          fi
```

**Critérios de Aceitação:**

* Workflow dispara em `git push origin v0.1.0` na main do fork.
* 3 imagens em ECR com tags `v0.1.0` + `latest` + SHORT_SHA.
* Scan retorna sem CRITICAL (ou findings documentados em allowlist).
* Tempo total < 10 min com cache quente.

**Riscos:**

* Build ARM em runner x86 usa QEMU (lento) — aceitar +5 min.
* ECR scan flaga CRITICAL em alpine — atualizar base ou allowlist documentada.
* Cross-org OIDC mis-config (ANU-115) bloqueia primeiro release — smoke em branch antes da tag oficial.

---

### ANU-117 — Patch `REDIS_DB` env var no fork + PR upstream

- **Track:** Bootstrap (este repo)
- **Branch:** `caiolcolaco/anu-117-patch-redis-db-env-var-fork-bataudit`
- **Dependência:** —
- **Refs:** este doc §3

**Objetivo:** suporte a `REDIS_DB` env var no Go (`internal/config/`, `internal/queue/`) com default 0; PR upstream `joaovrmoraes/bataudit`; merge no fork independente da decisão upstream.

**Atividades:**

* `grep -rn "REDIS_ADDRESS\|redis.NewClient" internal/ cmd/`
* Adicionar campo `RedisDB int` em `Config` struct + parser env var
* Passar `DB: cfg.RedisDB` para `redis.NewClient(&redis.Options{...})`
* Atualizar README + docker-compose.yml templates
* `go build ./... && go test ./...`
* Commit no fork → merge na main
* Abrir PR upstream

**Por que:** o EC2 BatAudit (ANU-107) usa Redis em DB lógico próprio para isolar dados BatAudit de qualquer Redis compartilhado futuramente. Mesmo que hoje BatAudit rode com Redis dedicado em container próprio, manter `REDIS_DB` configurável é boa prática + cidadania OSS (PR upstream).

**Critérios de Aceitação:**

* `go build ./...` + `go test ./...` passam
* README atualizado
* PR upstream aberto (URL anexada)
* Merge no fork independente da decisão upstream

---

## 2. Sync com upstream

Procedure manual periódico para manter o fork atualizado:

```bash
# Adicionar remote upstream uma vez
git remote add upstream https://github.com/joaovrmoraes/bataudit.git

# Sync periódico (sugerido: mensal ou ad-hoc quando upstream lança feature relevante)
git fetch upstream
git checkout -b chore/sync-upstream-$(date +%Y-%m-%d)
git merge upstream/main
# resolver conflitos (provavelmente em internal/config/ por causa de ANU-117)
git push origin chore/sync-upstream-...
gh pr create --title "chore: sync upstream joaovrmoraes/bataudit" --body "Merge upstream main into fork"
```

**Cadência sugerida:** mensal. Calendar reminder configurado.

**Após sync:** se houver mudanças em `sdks/node/src/`, **abrir PR no `anulamulta-platform`** atualizando `packages/core/src/bataudit/` (ou esperar ANU-106 — workflow `sync-bataudit-sdk.yml` automático — fazer isso semanalmente).

---

## 3. Quando este doc precisa ser atualizado

* Upstream lança feature que muda contrato do SDK Node — atualizar §0 + criar issue no AnulaMulta para vendor sync
* Caio decide publicar o SDK em npm público (caso outro projeto do grupo precise consumir) — adicionar BAT-1, 2, 3 (npm org + workflow + primeiro publish) e atualizar contracts no monorepo
* BatAudit migra para conta AWS dedicada (Caminho D) — substituir esta seção por procedure de migração

---

## 4. Referências

- Doc consumidor canônico: `anulamulta-platform/docs/ETAPA_2_IMPLEMENTATION_PLAN.md` §"Track BatAudit (ANU-105 a ANU-117)"
- Doc histórico (decisões): `anulamulta-platform/docs/TECHNICAL_AUDIT_AND_EVOLUTION_PLAN.md` §8.4
- Upstream: https://github.com/joaovrmoraes/bataudit

---

**Última atualização:** 2026-05-04 — escopo reduzido após decisão Caminho G (monorepo integrado).
