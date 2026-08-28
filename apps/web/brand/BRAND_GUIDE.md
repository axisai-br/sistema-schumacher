# Schumacher Tur — Brand Guide para Web

## 1. Autoridade

Este documento traduz a identidade oficial da Schumacher Tur para uso no
frontend web.

Fontes, em ordem de autoridade:

1. `reference/manual-identidade-visual-schumacher-tur-2023.pdf`
   - fonte principal e normativa;
2. `reference/apresentacao-schumacher-tur-2023.pdf`
   - contexto criativo e estratégico.

Em caso de divergência, o Manual de Identidade Visual prevalece.

Este arquivo não substitui os PDFs. Ele funciona como contrato operacional
para desenvolvimento web.

---

## 2. Conceito da marca

### Fatos do manual

A Schumacher Tur atua no transporte interestadual e estadual de
trabalhadores, atletas e turistas.

A marca enfatiza atendimento, satisfação, facilidades, conforto e segurança.

DNA declarado:

- Dinâmico
- Energético
- Segurança
- Inovação
- Profissional
- Qualidade
- Criatividade
- Flexibilidade
- Adaptabilidade

A identidade deve comunicar principalmente segurança, dinamismo,
profissionalismo, confiança e movimento.

O projeto também busca utilizar cores associadas visualmente a metais.

### Direção para web

O site deve parecer uma empresa moderna de mobilidade e transporte confiável.

Evitar estética genérica de:

- agência turística;
- SaaS;
- fintech;
- landing page baseada apenas em gradientes;
- luxo genérico desconectado da identidade oficial.

---

## 3. Paleta oficial

### Primárias

| Nome                | HEX       |
| ------------------- | --------- |
| Medium Gunship Grey | `#414951` |
| Emberglow           | `#DF674A` |

### Secundárias

| Nome            | HEX       |
| --------------- | --------- |
| Heavy Blue Grey | `#A0A9AF` |
| Welcoming Wasp  | `#E8B100` |
| Alluvial Inca   | `#A08B63` |
| Lynx White      | `#F0F5F7` |

Não substituir essas cores por aproximações quando forem usadas como cores
oficiais da marca.

### Adaptação semântica para web

Uso recomendado, não imposto literalmente pelo manual:

- `#414951` — texto institucional, superfícies escuras, contraste;
- `#DF674A` — ação principal, CTA e destaque da marca;
- `#F0F5F7` — superfície clara principal;
- `#A0A9AF` — bordas, textos secundários e elementos discretos;
- `#A08B63` — acento secundário de caráter metálico/premium;
- `#E8B100` — destaque secundário controlado.

As seis cores não devem competir com a mesma importância na mesma tela.

---

## 4. Tipografia

### Fato do manual

A tipografia oficial da marca é **Franie**.

O manual associa a Franie a:

- modernidade;
- inovação;
- profissionalismo;
- energia;
- linhas limpas;
- curvas suaves;
- experiência digital.

### Regra de implementação

Não baixar, copiar ou fabricar Franie sem asset oficial e licença compatível.

Usar fallback claramente tratado como adaptação técnica, nunca como
substituição oficial:

`Franie, Outfit, Inter, sans-serif`

### Estado de implementação

Os arquivos oficiais fornecidos para este projeto estão integrados sem
conversão ou alteração, com `font-display: swap`:

- `Franie-SemiLight.otf` — peso `400`;
- `Franie-SemiLightItalic.otf` — peso `400`, estilo `italic`;
- `Franie-Regular.otf` — peso `500`;
- `Franie-SemiBold.otf` — peso `600`;
- `Franie-Bold.otf` — peso `700`.

Franie Black e as demais variantes itálicas permanecem somente no acervo-fonte
local. `Franie-SemiLightItalic.otf` entra no runtime para o uso `italic`
existente nos depoimentos.

---

## 5. Logotipo

### Conceito

O símbolo combina:

- letra S;
- ideia de sol/espiral;
- movimento e dinamismo;
- força e confiança.

Na assinatura tipográfica existem personalizações próprias. O manual destaca,
entre outras, a relação visual de T e U formando um elemento de proteção/teto.

### Construção

O logotipo prioriza traços marcantes e poucos detalhes para preservar sua
leitura em redução.

### Área de proteção

O manual define `1X` como referência para a margem de respiro da aplicação.

Nenhum elemento externo deve invadir essa área quando a assinatura ou símbolo
forem utilizados.

### Redução

O manual apresenta limites de redução para web de:

- `180 px`;
- `50 px`;

aplicados às respectivas versões demonstradas no manual.

A implementação deve conferir visualmente no documento qual variante
corresponde a cada limite antes de aplicá-lo.

### Mapeamento implementado

- Header a partir de `sm`: assinatura horizontal escura (`LOGO (2)`), com
  largura visual mínima de `180 px`;
- Header abaixo de `sm`: símbolo escuro (`LOGO (7)`), com largura visual mínima
  de `50 px`;
- Footer: assinatura horizontal clara (`LOGO (3)`);
- favicon: símbolo escuro (`LOGO (7)`).

Os arquivos selecionados recebem nomes semânticos no runtime. O inventário
bruto, com nomes originais preservados, fica em
`brand/source-assets.local/`, diretório local ignorado pelo Git.

### Uso correto

As versões principais e secundárias devem usar somente as combinações de
cores, fundos, proporção e diagramação previstas pelo manual.

É proibido:

- deformar;
- esticar;
- comprimir;
- recolorir arbitrariamente;
- alterar proporção entre símbolo e lettering;
- reconstruir a assinatura com texto HTML/CSS;
- redesenhar o símbolo;
- criar uma nova versão do logo.

Texto comum "Schumacher Tur" pode existir na interface, mas não deve ser
apresentado como substituto da assinatura oficial.

---

## 6. Grafismos

### Fato do manual

Os grafismos são elementos de apoio destinados a reforçar consistência e
reconhecimento da marca.

São derivados geometricamente do símbolo S e aparecem em:

- formas preenchidas;
- formas outline;
- patterns.

### Regra para web

Não inventar grafismos novos que se apresentem como oficiais.

### Mapeamento implementado

- `GRAFISMO (1).svg`: grafismo neutro usado no FinalCTA;
- `GRAFISMO (4).svg`: grafismo Ember usado na página de orçamento.

As cópias de runtime recebem nomes semânticos e preservam os arquivos oficiais
sem edição, filtro, recoloração ou deformação. `GRAFISMO (2).svg`, `PATTERN.svg`
e os demais grafismos permanecem somente no acervo-fonte local até existir uso
comprovado na interface.

---

## 7. Sistema de layout

### Fato do manual

O manual define um grid base de `4 × 4`.

O destaque principal deve ser a imagem.

As proporções apresentadas incluem:

- texto ocupando até `2/4` do grid;
- logotipo ocupando `1/4` ou `2/4`;
- imagem ocupando metade ou todo o grid;
- texto podendo ser aplicado sobre grafismo.

### Adaptação para web

O grid 4×4 deve ser entendido como princípio compositivo, não como obrigação de
criar literalmente quatro colunas CSS em todos os breakpoints.

Na web:

- imagens devem ter presença visual forte;
- conteúdo deve manter áreas generosas de respiro;
- texto não deve dominar toda a superfície;
- layouts podem adaptar a proporção conforme viewport;
- mobile pode reorganizar a composição sem perder hierarquia.

---

## 8. Imagens

### Direção derivada do manual

A imagem possui papel dominante no sistema de layout.

Para o site, priorizar quando disponíveis:

- frota real;
- veículos;
- experiência de transporte;
- estrada e destinos;
- passageiros somente quando houver material autorizado.

Evitar imagens genéricas que não representem a operação real da Schumacher.

---

## 9. Movimento

Movimento e dinamismo pertencem ao DNA da marca.

Na web isso pode aparecer por:

- composição;
- direção das formas;
- fotografia;
- transições discretas;
- animações funcionais.

Movimento não significa adicionar animação decorativa indiscriminadamente.

Toda animação deve preservar `prefers-reduced-motion`.

---

## 10. Componentes web

### Header

Deve priorizar:

- marca;
- navegação clara;
- confiança;
- CTA principal identificável.

Usar somente logo/símbolo oficial disponível.

### Hero

Deve comunicar rapidamente:

- Schumacher Tur;
- transporte;
- segurança;
- conforto;
- dinamismo;
- profissionalismo.

A identidade deve vir de tipografia, composição, imagem e cores oficiais,
não de efeitos decorativos genéricos.

### CTAs

Adaptação recomendada:

- primário: Emberglow;
- secundário: outline com Gunship Grey ou Emberglow;
- foco visível obrigatório.

### Cards

Devem manter aparência sóbria, boa hierarquia e contraste.

Evitar excesso de:

- sombras;
- gradientes;
- bordas arredondadas;
- efeitos hover decorativos.

---

## 11. Acessibilidade e responsividade

A aplicação da marca nunca pode remover:

- navegação por teclado;
- focus-visible;
- contraste adequado;
- reduced-motion;
- touch targets adequados;
- semântica HTML;
- responsividade.

Não usar `overflow-x: hidden` globalmente apenas para esconder problemas de
layout.

---

## 12. Separação entre autoridade e criação

### Fatos oficiais

São fatos e não devem ser reinterpretados:

- identidade visual definida no manual;
- paleta;
- Franie;
- conceito do símbolo;
- versões do logo;
- área de proteção;
- limites de redução;
- grafismos derivados do S;
- grid visual 4×4;
- DNA da marca.

### Adaptações para web

Podem ser decididas pelo frontend:

- tokens semânticos;
- escala responsiva;
- spacing;
- comportamento do grid em diferentes viewports;
- estados de hover/focus;
- hierarquia entre cores;
- fallback tipográfico;
- intensidade de animações.

### Sugestões criativas

Qualquer solução não explicitamente definida no manual deve ser tratada como
decisão criativa e não como regra oficial.

Uma decisão criativa nunca pode contradizer uma regra explícita do manual.

---

## 13. Invariantes do track

Toda implementação de identidade visual deve preservar:

- comportamento funcional;
- rotas;
- integrações;
- conteúdo comercial existente, salvo autorização específica;
- responsividade;
- acessibilidade;
- reduced-motion;
- performance razoável.

Não transformar branding em refatoração arquitetural.

Não:

- inventar nova identidade;
- redesenhar logo;
- inventar cores oficiais;
- inventar grafismos oficiais;
- baixar Franie sem autorização/licença;
- adicionar dependência sem necessidade comprovada;
- alterar backend ou infraestrutura.
