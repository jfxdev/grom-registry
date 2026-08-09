# Plano de entrega: paginação e aceitação browser de fluxos destrutivos

## Status e objetivo

**Status: parcialmente implementado (9 de agosto de 2026).**

Esta é uma entrega única. Ela fecha as lacunas restantes da experiência de
administração ao:

1. substituir listas de crescimento ilimitado por paginação estável baseada em
   cursor; e
2. ampliar a aceitação Playwright do produto instalado para as ações de maior
   impacto sobre acesso, credenciais, metadados e artefatos.

O escopo é o caminho padrão suportado: uma instância Grom, SQLite,
armazenamento local, Distribution privado e clientes Docker. Não introduz
PostgreSQL como capacidade anunciada, S3, ORAS, auditoria navegável,
integrações ativas ou mudanças ao protocolo OCI.

O trabalho deve preservar as decisões em
[`architecture-and-mvp.md`](architecture-and-mvp.md): Grom continua sendo o
único ponto público; Distribution continua sem modificações; a primeira parte
do nome do repositório continua sendo a fronteira de projeto; exclusão de
manifesto continua a resolver por digest através do plano de controle; e
coleta de blobs continua uma operação distinta.

## Estado atual

`backend/internal/foundation/pagination.go` define os tipos independentes de
persistência e transporte `PageRequest`, `PageResult[T]` e `PageCursor`.
Backups já expõem paginação opaca e fixa em cinco recovery points. As listas
administrativas e de repositórios já possuem a primeira entrega de cursor.
Tags, inventário de manifests, histórico de exclusões e execuções de lifecycle
agora usam páginas: tags delegam a continuação para Distribution e as três
listas persistidas usam keyset por timestamp/ID. A ampliação da aceitação
Playwright destrutiva permanece pendente.

O Playwright já tem uma jornada pública instalada em
`frontend/e2e/admin-journey.spec.ts`: ele inicia um projeto Compose isolado,
usa somente a URL pública, cria recursos pelo UI e usa Docker apenas para
enviar a imagem de fixture. Essa infraestrutura será reutilizada. Não criar um
segundo harness, não consultar o banco, não alcançar a porta privada do
Distribution e não executar limpeza Docker ampla.

## Contrato de paginação

### Convenções comuns

Toda lista incluída nesta entrega passa a aceitar:

- `cursor`: opcional, opaco e retornado somente pelo servidor;
- `limit`: opcional, com padrão `25` e máximo `100`.

Toda resposta paginada usa a forma:

```json
{
  "items": [],
  "nextCursor": "opaque-server-value"
}
```

Não expor `offset`, contagem total ou valores internos de ordenação. O cursor
é validado no servidor; cursor malformado, incompatível com filtros ou maior
que o limite documentado retorna `400 invalid_cursor`. Uma página sem próxima
página omite `nextCursor`.

Cursores são keyset cursors: cada fonte define uma ordenação total e inclui um
desempate estável. Para registros persistidos, a ordem padrão é
`created_at DESC, id DESC`; para dados cujo significado exige outra ordem, a
ordem está explicitada abaixo. A página seguinte não pode repetir elementos da
anterior quando não há escrita concorrente. Sob escrita concorrente, o cursor
preserva a ordenação definida e não promete uma visão transacional global.

O OpenAPI é alterado primeiro. A geração Go/TypeScript ocorre com `make
generate`; arquivos gerados permanecem sem edição manual. Handlers e DTOs não
vazam modelos Bun para os contextos de domínio.

### Filtros e interface

Busca e filtro que hoje são executados sobre listas carregadas por inteiro
passam a ser parâmetros de servidor antes da paginação:

- usuários: `q`;
- service accounts: `q` e `status=active|disabled|all`;
- demais listas: os filtros de escopo já existentes, como `project` e
  `repository`, continuam obrigatórios quando aplicáveis.

Mudar busca, filtro ou escopo descarta o cursor atual e reinicia a primeira
página. A interface mostra controles acessíveis **Anterior** e **Próxima**;
mantém uma pilha local dos cursores visitados para voltar sem tentar decodificar
o cursor, e desabilita controles indisponíveis. Estados vazio, carregando e
erro permanecem claros. A UI não exibe total como se representasse toda a
instalação quando somente uma página está carregada.

### Recursos incluídos

| Fatia | Endpoints e recursos | Ordenação e particularidades |
|---|---|---|
| Administração | Usuários, service accounts, tokens de service account e memberships | Usuários, contas e tokens usam criação/ID; memberships usam `principal_kind, principal_id`. Busca e status filtram no servidor. |
| Navegação | Projetos e repositórios lógicos/descobertos | Projetos usam criação/ID. Repositórios usam nome/ID após a reconciliação com o catálogo. O cliente Distribution deve consumir a paginação de catálogo em vez de carregar o catálogo inteiro. |
| Registro | Tags, inventário de manifests, histórico de exclusões e lifecycle runs | Tags usam ordem lexicográfica e paginação nativa do Distribution encapsulada em cursor Grom. Inventário, exclusões e runs usam criação/ID. |

Policy presets e integrações são catálogos pequenos controlados pelo produto e
ficam fora desta entrega. Backups preservam o contrato existente de cinco itens
por página, inclusive os seus cursores e garantias de recuperação.

As respostas de reconciliação e dry-run continuam operações pontuais, não
listas navegáveis: paginar apenas suas respostas esconderia resultados de uma
operação que precisa ser apresentada por inteiro. A listagem posterior de
inventário e de runs é que recebe paginação.

## Implementação por camada

Para cada recurso paginado:

1. Alterar `backend/api/openapi.yaml` com parâmetros, respostas paginadas e
   erros `400` para cursor inválido.
2. Executar `make generate` e adaptar somente o código de aplicação e de
   transporte que consome os tipos gerados.
3. Trocar a capacidade de listagem no repositório do contexto proprietário por
   uma capacidade paginada. A implementação Bun mantém o cursor e a ordenação
   no contexto de infraestrutura, não em handlers HTTP.
4. Adaptar serviços de aplicação e handlers em `backend/internal/httpapi/`.
5. Para catálogo e tags, estender o adaptador em
   `backend/internal/registry/infrastructure/distribution/` para seguir as
   páginas de Distribution sem permitir que detalhes do seu cabeçalho `Link`
   atravessem o contrato Grom.
6. Adaptar clientes em `frontend/src/modules/*/api`, chaves TanStack Query e
   telas. Cada query key inclui cursor e filtros relevantes.
7. Cobrir repositório, serviço, HTTP, cliente e componente com primeira
   página, próxima página, retorno, cursor inválido e alteração de filtro.

## Aceitação browser dos fluxos destrutivos

Adicionar `frontend/e2e/admin-destructive-flows.spec.ts`. As jornadas usam o
mesmo global setup de `admin-journey.spec.ts`, mas cada caso recebe nomes de
projeto, usuário e service account próprios. A suíte permanece serializada.
Segredos reveal-once existem somente em memória ou em diretórios Docker
temporários; traces, vídeos e screenshots continuam desligados para o modo
administrativo público.

| Jornada | Ações e asserções públicas |
|---|---|
| Remover membership | Criar e associar uma service account pelo UI, abrir a confirmação, remover e verificar que a linha desaparece. A prova de perda de escopo no próximo token exchange continua coberta pelo Registry E2E. |
| Desabilitar service account e revogar chave | Exigir a confirmação textual para desabilitar, verificar o estado `Disabled` e a remoção da chave revogada na lista. A prova de token inválido permanece no E2E de protocolo. |
| Exclusão manual de artefato | Enviar fixture Docker, abrir a prévia de exclusão, conferir digest/tags afetadas, confirmar e verificar atualização de tags e histórico. |
| Lifecycle manual | Enviar duas imagens distintas, configurar pelo UI uma retenção que deixe uma elegível, revisar o dry-run, fornecer motivo, executar e verificar resultado, tags e inventário atualizados. |
| Arquivar e remover registro lógico | Arquivar e remover pelo UI um repositório lógico vazio. Para repositório com conteúdo, confirmar o bloqueio e que nenhum conteúdo OCI é removido. |
| Desabilitar usuário | Criar usuário não administrador, autenticá-lo em um segundo contexto de navegador, desabilitá-lo no contexto administrador e confirmar que a sessão alvo deixa de ser válida. |
| Excluir projeto | Excluir projeto vazio com sucesso; para projeto com repositório, confirmar a resposta de conflito e preservação do projeto. |
| Excluir recovery point | Criar recovery point no stack isolado, exigir a digitação de `DELETE`, excluir e verificar que o item sai da listagem. O teste de desastre/restore completo continua em sua suíte especializada. |

Cada jornada deve usar papéis, rótulos e nomes visíveis para seleção. Adicionar
`data-testid` somente quando a semântica não produzir um seletor estável. Uma
assertiva de UI deve ser acompanhada pela resposta HTTP pública relevante e por
uma atualização visível da tela; não basta confirmar que o modal fechou.

## Ordem interna da entrega

1. Implementar a base comum de cursor, validação e controles de página.
2. Implementar paginação administrativa e mover busca/filtro de usuários e
   service accounts ao servidor.
3. Adicionar E2E de membership, service account/chave e exclusão de artefato.
4. Implementar paginação de projetos e repositórios, incluindo o adaptador de
   catálogo Distribution.
5. Adicionar E2E de lifecycle e de arquivamento/remoção lógica.
6. Implementar paginação de tags, inventário, exclusões e lifecycle runs.
7. Adicionar E2E de usuário, projeto e recovery point.
8. Executar a validação completa e registrar a evidência de aceitação.

## Critérios de aceite

### Paginação

- Nenhum endpoint incluído retorna uma coleção ilimitada.
- Cursores inválidos ou incompatíveis retornam erro de validação sem consulta
  não limitada.
- A próxima página não repete elementos da anterior na ausência de escrita
  concorrente.
- Busca e filtros administrativos cobrem o conjunto inteiro, não apenas a
  página já carregada.
- Tags e catálogos grandes não obrigam Grom a montar uma lista completa antes
  de responder à primeira página.
- A interface oferece navegação por teclado, estados de carregamento/erro e
  controles corretos para primeira, intermediária e última página.

### Fluxos destrutivos

- Todo fluxo listado acima usa a UI servida pelo container Grom e somente sua
  superfície pública.
- Confirmações obrigatórias impedem a mutação até que sejam satisfeitas.
- Sucesso e conflitos atualizam a interface e exibem a consequência correta;
  conteúdo OCI não é apagado quando a regra de domínio determina bloqueio.
- Chaves, senhas, cookies, URLs de reset e segredos reveal-once não aparecem
  em argumento de processo, diagnóstico, artefato de browser ou log.
- `make test-admin-e2e` cobre a suíte ampliada e mantém a limpeza restrita ao
  projeto Compose, imagens e diretórios temporários que a própria execução
  criou.

## Verificação e documentação de encerramento

Executar, no mínimo:

```text
make generate
make test
make build
make test-admin-e2e
make test-registry-e2e
```

Executar também os checks Docker de release aplicáveis antes de promover uma
release. Ao concluir, atualizar `docs/architecture-and-mvp.md` e
`docs/mvp-acceptance-implementation-plan.md` com data, comando e evidência.
Atualizar `docs/code-map.md` quando o novo spec ou módulo de paginação for
introduzido. `docs/domain-model.md` só requer mudança se a entrega criar um
novo tipo arquitetural além de `foundation.PageRequest` e
`foundation.PageResult[T]`; a intenção deste plano é reutilizá-los.
