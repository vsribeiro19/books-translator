# MVP SCOPE — PDF Book Translator

## O que ESTÁ no MVP

### Must Have
| Item | Descrição |
|---|---|
| Upload de PDF nativo | Aceita PDF com texto extraível (sem OCR), até ~800 páginas |
| Extração estruturada | Extrai texto respeitando ordem de leitura, capítulos, parágrafos e títulos |
| Tradução EN → PT-BR | Via API de LLM configurável, em chunks, mantendo contexto entre blocos de um mesmo capítulo |
| Motor de LLM plugável | Configuração externa (env/config), começando com DeepSeek V4 Flash Free via OpenCode Zen |
| Reconstrução do PDF | Gera novo PDF com diagramação limpa e legível (não é edição in-place do original) |
| Fluxo padrão assíncrono | Processa o livro inteiro e entrega o PDF completo ao final |
| Flag de prévia (`preview_first_chapter`) | Se marcada, traduz e entrega o primeiro capítulo antes de finalizar o restante |
| Paralelismo por capítulo | Orchestrator em Go dispara goroutines para processar capítulos simultaneamente |
| Endpoint de status | Permite consultar o estado do job (pending/processing/preview_ready/completed/failed) |

### Should Have
| Item | Descrição |
|---|---|
| Indicador de progresso mais granular | Ex: "3 de 12 capítulos traduzidos" |
| Preservação aproximada de imagens | Posição relativa mantida no fluxo do texto reconstruído |

### Could Have
| Item | Descrição |
|---|---|
| Interface web simples | Hoje é só API; UI fica para versão futura |
| Detecção de capítulo via sumário do PDF | Usar índice/bookmarks do PDF quando existirem, em vez de só heurística visual |

## O que NÃO está no MVP (Future Scope)

- OCR para PDFs escaneados
- Tradução em tempo real / streaming palavra a palavra
- Outros idiomas além de inglês → português
- Suporte a EPUB ou outros formatos de entrada
- App mobile
- Autenticação e suporte multiusuário
- Preservação pixel-perfect do layout visual original (colunas, posições exatas)
- Fila de mensagens / infraestrutura distribuída (Go e Node se comunicam via HTTP simples)

## Justificativa das Decisões de Escopo

- **Reconstrução em vez de edição in-place**: tradução EN→PT-BR gera texto ~15-25% mais longo, o que causa overflow e sobreposição em abordagens de edição direta do PDF original. Reconstruir evita esse problema e é mais confiável para livros longos.
- **Prévia como flag opcional, não padrão**: entrega progressiva por capítulo adiciona complexidade de estado e remontagem incremental de PDF. Torná-la opt-in evita forçar essa complexidade em todo processamento, mantendo o caminho padrão simples.
- **Sem OCR**: os PDFs reais de uso do usuário são nativos; adicionar OCR aumentaria escopo sem necessidade validada.
- **LLM plugável desde o início**: o free tier do DeepSeek via OpenCode Zen é tratado pelo próprio provedor como promocional, sujeito a mudança. Desacoplar o motor de tradução evita retrabalho se isso mudar.
- **Sem fila de mensagens**: para volume de uso pessoal (até 2 livros/mês), HTTP direto entre Go e Node é suficiente e reduz complexidade operacional.

## Hipóteses a Validar com o MVP

1. A extração + reconstrução mantém a estrutura de leitura de forma satisfatória mesmo em livros longos e com diagramação variada.
2. A detecção heurística de capítulos é confiável o suficiente para a prévia fazer sentido na prática (sem falsos positivos constantes).
3. A qualidade de tradução via LLM (começando com DeepSeek V4 Flash Free) é satisfatória para conteúdo literário e acadêmico.
4. O tempo de processamento total para um livro de ~800 páginas é aceitável para uso real.
5. O paralelismo via goroutines de fato acelera o processamento sem esbarrar em rate limits do provedor de LLM.

## Métricas de Sucesso do MVP

- Pelo menos um livro de 200+ páginas e um de 700+ páginas processados de ponta a ponta sem falhas críticas
- PDF final abre corretamente em pelo menos dois leitores diferentes (ex: Kindle e Adobe Reader/preview)
- Zero trechos não traduzidos no resultado final (fora de casos marcados como erro)
- Tempo entre solicitação de prévia e entrega do primeiro capítulo sensivelmente menor que o tempo total do job completo