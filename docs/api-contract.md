# Contrato de API — PDF Book Translator

Serviços:
- **Orchestrator (Go)** — `http://localhost:8080`
- **pdf-service (Node)** — `http://localhost:8081`

Formato de dados compartilhado entre os serviços (JSON), usado pelo pdf-service em `/extract` e `/rebuild` e persistido pelo orchestrator.

## Estrutura compartilhada

```
Block
  type: "heading" | "paragraph"
  level: number        # heading: hierarquia (1 = capítulo, 2+ = subseção); paragraph: ignorado
  text: string

Chapter
  title: string
  blocks: Block[]

ExtractResult
  pageCount: number
  chapters: Chapter[]
```

## pdf-service

### `POST /extract`
Recebe o PDF como `multipart/form-data`, campo **`pdf`**.
- Sucesso: `200` → corpo `ExtractResult` (JSON).
- PDF corrompido/protegido por senha: `400` → `{ "error": "invalid_pdf", "message": "..." }`.
- PDF sem texto extraível (escaneado): `422` → `{ "error": "no_text", "message": "..." }`.

### `POST /rebuild`
Recebe JSON: `{ "title": string, "chapters": Chapter[] }`.
- Sucesso: `200` → `application/pdf` (arquivo no corpo).
- Estrutura inválida: `400` → `{ "error": "invalid_structure", "message": "..." }`.

## Orchestrator

### `POST /jobs`
`multipart/form-data`: campo **`pdf`** + campo opcional **`preview_first_chapter`** (`"true"`/`"false"`).
- Sucesso: `202` → `{ "jobId": string, "status": "pending" }`.
- Validação falhou: `400` → `{ "error": "...", "message": "..." }`.

### `GET /jobs/{id}`
- Sucesso: `200` → `{ "jobId": string, "status": "pending|processing|preview_ready|completed|failed", "error?": string, "progress?": { "chaptersDone": number, "chaptersTotal": number } }`.

### `GET /jobs/{id}/preview`
PDF parcial (primeiro capítulo), disponível quando `status = preview_ready`.
- `200` → `application/pdf`
- `409` → ainda não pronto; `404` → job inexistente.

### `GET /jobs/{id}/result`
PDF completo, disponível quando `status = completed`.
- `200` → `application/pdf`
- `409` → ainda não pronto; `404` → job inexistente.

## Status do job

| Status | Significado |
|---|---|
| `pending` | recebido, aguardando processamento |
| `processing` | extração/tradução em andamento |
| `preview_ready` | prévia do 1º capítulo disponível |
| `completed` | PDF completo disponível |
| `failed` | falhou; `error` descreve o motivo |
