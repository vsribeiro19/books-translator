# PRD — PDF Book Translator

## 1. Visão Geral do Produto

Sistema de tradução de PDFs nativos (inglês → português) para uso pessoal, composto por dois serviços que se comunicam via HTTP:

- **Orchestrator (Go)** — recebe o job de tradução, coordena a extração, divide o conteúdo em capítulos, dispara o processamento em paralelo (goroutines) por capítulo, chama a API de LLM para tradução, gerencia o estado do job e decide quando a prévia (se solicitada) está pronta.
- **PDF Service (Node/TypeScript)** — responsável por duas operações: (a) extrair texto estruturado de um PDF original, respeitando ordem de leitura, títulos e parágrafos; (b) reconstruir um novo PDF a partir do texto traduzido, com diagramação limpa.

O fluxo é assíncrono: o usuário envia o PDF, o sistema processa em background, e o resultado (parcial ou completo) fica disponível para download.

## 2. Persona

**Persona única: o próprio usuário/desenvolvedor**
- Perfil: desenvolvedor que lê livros e artigos acadêmicos em inglês baixados da internet, para consumo no Kindle, celular ou computador.
- Job to be done: "quero ler esse conteúdo em português, sem esperar por ferramentas de terceiros com limitações de tamanho ou custo, e sem perder a legibilidade do texto."
- Resultado ideal: abrir o PDF traduzido e conseguir ler como se fosse o material original, com estrutura de capítulos e parágrafos preservada.

## 3. User Stories

- Como usuário, quero enviar um PDF em inglês para o sistema, para receber de volta o mesmo conteúdo traduzido para português.
- Como usuário, quero que o sistema aceite livros grandes (até 800 páginas), para não esbarrar nas limitações das ferramentas comerciais existentes.
- Como usuário, quero optar por receber uma prévia do primeiro capítulo traduzido antes da finalização, para começar a ler enquanto o restante processa.
- Como usuário, quero que o PDF final mantenha a estrutura de capítulos e parágrafos do original, para ter uma experiência de leitura confortável.
- Como usuário, quero poder trocar o provedor de LLM usado na tradução (ex: DeepSeek gratuito, Claude, GPT), para não depender de um único serviço que pode mudar de política a qualquer momento.
- Como usuário, quero consultar o status de um job de tradução em andamento, para saber se já posso baixar o resultado (parcial ou completo).

## 4. Requisitos Funcionais

### 4.1 Upload e Validação (Orchestrator)
- Endpoint para receber upload de arquivo PDF
- Validação: arquivo deve ser PDF nativo (conter texto extraível, não apenas imagens escaneadas)
- Validação: tamanho/página dentro de limites técnicos razoáveis (a definir em teste, referência inicial: até 800 páginas)
- Parâmetro opcional na requisição: `preview_first_chapter` (boolean)

### 4.2 Extração de Texto Estruturado (PDF Service)
- Endpoint `/extract` que recebe o PDF e retorna o texto segmentado por capítulo/seção, respeitando ordem de leitura
- Detecção de limites de capítulo (por heurística: tamanho de fonte de títulos, quebras de página, ou sumário/índice do PDF quando disponível)
- Preservação da posição relativa de parágrafos e títulos na estrutura retornada

### 4.3 Orquestração e Tradução (Orchestrator)
- Divisão do conteúdo extraído em chunks por capítulo
- Se `preview_first_chapter = true`: prioriza tradução do primeiro capítulo e libera esse resultado antes de continuar com o restante
- Se `preview_first_chapter = false` (padrão): processa todos os capítulos e só entrega o resultado ao final
- Paralelismo via goroutines: múltiplos capítulos podem ser traduzidos simultaneamente, respeitando limites de rate/contexto do provedor de LLM configurado
- Chamada à API de LLM configurada (motor plugável — endpoint/modelo definidos via configuração, não hardcoded)
- Manutenção de contexto entre chunks de um mesmo capítulo (quando o capítulo excede o limite de tokens em uma única chamada)
- Tratamento de falha: se uma chamada de tradução falhar, o sistema deve tentar novamente (retry) antes de marcar o capítulo como erro

### 4.4 Reconstrução do PDF (PDF Service)
- Endpoint `/rebuild` que recebe o texto traduzido estruturado (por capítulo/parágrafo/título) e gera um novo arquivo PDF
- Diagramação limpa e legível (fonte, espaçamento e hierarquia de títulos consistentes) — não é exigida réplica pixel-a-pixel do PDF original
- Posição aproximada de imagens preservada no fluxo do texto (should have, não bloqueia o MVP se não sair no primeiro corte)

### 4.5 Status e Entrega
- Endpoint de consulta de status do job (ex: `pending`, `processing`, `preview_ready`, `completed`, `failed`)
- Quando `preview_first_chapter = true`: disponibiliza um PDF parcial (primeiro capítulo) para download assim que pronto, e um PDF completo depois
- Quando `preview_first_chapter = false`: disponibiliza apenas o PDF completo ao final

## 5. Requisitos Não-Funcionais

- **Performance**: processamento assíncrono obrigatório — nenhuma chamada deve bloquear esperando o livro inteiro ser traduzido de forma síncrona
- **Escalabilidade não é objetivo**: sistema é para uso individual (sem necessidade de suportar múltiplos usuários simultâneos, mas o design não deve impedir isso no futuro)
- **Plugabilidade de LLM**: configuração do provedor de tradução (URL base, chave de API, modelo) deve ser externa ao código (variáveis de ambiente/arquivo de config), permitindo troca sem alteração de lógica de negócio
- **Resiliência**: falha em um capítulo não deve derrubar o processamento dos demais capítulos
- **Sem autenticação/multiusuário no MVP** — endpoint de uso local/pessoal
- **Comunicação entre serviços**: HTTP simples (REST) entre Go e Node; sem fila de mensagens no MVP

## 6. Integrações Necessárias

- **Provedor de LLM para tradução**: inicialmente DeepSeek V4 Flash Free via OpenCode Zen (endpoint compatível com formato OpenAI), com possibilidade de troca futura para outros provedores (Claude, GPT, DeepL, etc.)
- **Bibliotecas de PDF (Node)**: extração de texto estruturado e geração de novo PDF (ex.: `pdf-parse`/`pdfjs-dist` para extração, `pdfkit` ou `pdf-lib` para geração) — escolha final de biblioteca fica a critério da implementação
- Não há integrações de autenticação, storage externo ou banco de dados robusto exigidas no MVP (armazenamento local de arquivos é suficiente)

## 7. Casos de Borda / Edge Cases

- PDF sem estrutura de capítulos detectável (ex: artigo curto sem títulos claros) → sistema deve tratar o documento inteiro como um único "capítulo"
- Falsos positivos na detecção de capítulo (ex: subtítulo interpretado como novo capítulo) → aceitável no MVP, sem bloquear o processamento
- Capítulo maior que o limite de contexto do LLM configurado → deve ser subdividido em chunks menores com contexto compartilhado entre eles
- Falha na chamada da API de LLM (timeout, rate limit, erro do provedor) → retry automático; se persistir, capítulo é marcado como erro e o restante do job continua
- PDF protegido por senha ou corrompido → deve ser rejeitado na validação inicial com mensagem de erro clara
- PDF majoritariamente escaneado (sem texto extraível) → fora do escopo do MVP; deve ser rejeitado com mensagem indicando que OCR não é suportado
- Usuário solicita prévia (`preview_first_chapter = true`) em um documento sem capítulos detectáveis → sistema deve informar que a prévia não é aplicável e seguir com o fluxo padrão

## 8. Critérios de Aceitação

- **Upload**: o sistema aceita um PDF nativo de até 800 páginas e retorna um identificador de job
- **Extração**: o texto extraído preserva a ordem de leitura e a segmentação por capítulo/parágrafo em pelo menos os casos de teste com estrutura clara (títulos com hierarquia visual definida)
- **Tradução**: o conteúdo final está integralmente traduzido para português, sem trechos deixados no idioma original (exceto falhas registradas como erro)
- **Reconstrução**: o PDF final é aberto sem erros em leitores padrão (Adobe Reader, Kindle, preview do sistema operacional) e mantém hierarquia de títulos e parágrafos legível
- **Prévia**: quando solicitada, o PDF do primeiro capítulo fica disponível antes da conclusão do job completo, de forma perceptível (não "quase ao mesmo tempo")
- **Status**: é possível consultar o estado do job a qualquer momento durante o processamento