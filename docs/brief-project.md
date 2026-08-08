# BRIEF — PDF Book Translator

## Problema
Ferramentas de tradução de PDF existentes (DeepL, Adobe, Smallpdf, Reflo, DocuGlot) resolvem o problema de forma incompleta para uso pessoal: ou têm limitações de tamanho/páginas, ou exigem planos pagos, ou não entregam controle total sobre o processo. Não existe uma ferramenta pessoal, sem limites artificiais, para traduzir livros e artigos acadêmicos em PDF de inglês para português mantendo uma leitura confortável.

## Solução Proposta
Uma API pessoal que recebe um PDF nativo (texto selecionável, sem OCR) em inglês, extrai o conteúdo respeitando a estrutura de leitura (capítulos, parágrafos, títulos), traduz via LLM (motor plugável) e reconstrói um novo PDF traduzido com diagramação limpa e legível — sem tentar editar o PDF original "pixel a pixel", evitando os problemas de overflow de texto que essa abordagem causa em traduções EN→PT (texto ~15-25% mais longo).

Suporta um fluxo opcional de prévia: o usuário pode solicitar que o primeiro capítulo seja traduzido e entregue primeiro, enquanto o restante do livro continua processando em segundo plano.

## Público-alvo
Uso estritamente pessoal — o próprio criador do projeto, para traduzir livros e artigos acadêmicos que baixa para ler no Kindle, celular ou computador.

## Diferencial Competitivo
- Sem limite artificial de páginas ou tamanho de arquivo (testado para livros de 200–800 páginas)
- Reconstrução do PDF (não edição in-place) — evita bugs de layout comuns em ferramentas comerciais
- Motor de tradução (LLM) plugável — não depende de um único provedor pago
- Fluxo de prévia por capítulo, com paralelismo via goroutines (Go) para acelerar o processamento
- Projeto também serve como veículo de aprendizado (Go + Node trabalhando juntos)

## Modelo de Negócio
Não aplicável — ferramenta de uso pessoal, sem intenção de monetização.

## Métricas de Sucesso
- Livro de até 800 páginas processado e traduzido com texto legível de ponta a ponta, sem cortes ou sobreposições
- Detecção de capítulos funciona na maioria dos livros/artigos testados, sem falsos positivos frequentes
- Tempo de processamento total aceitável para uso real (referência a validar durante o MVP)
- Prévia do primeiro capítulo entregue significativamente mais rápido que o livro completo