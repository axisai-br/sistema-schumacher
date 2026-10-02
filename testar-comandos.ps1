<#
.SYNOPSIS
  Conversa com o bot usando o MOTOR POR COMANDOS (LLM so extrai; o codigo decide e responde por template).

.DESCRIPTION
  Mesmo simulador do testar-atendimento.ps1 (agente real + LLM real, dados de teste em memoria, nada vai
  para o banco nem para o WhatsApp), ja configurado como nas medicoes do motor por comandos:
  ATENDIMENTO_V2_MOTOR=comandos, roteador Jev, modelo pequeno da NVIDIA sem raciocinio, sem hedge.

  O motor por comandos exige o Jev: a TYPESAFE_API_KEY precisa estar em apps\api\.env.atendimento-local
  (o testar-atendimento.ps1 -NovasChaves pede e grava). As chaves nunca aparecem na tela.

  Dentro da conversa: digite como o cliente. "+texto" junta mensagens (picadas), /turno mostra o que o
  extrator e o roteador entenderam e o ato escolhido, /estado mostra a compra, /reservas os PIX,
  /reset recomeca, /sair encerra.

.PARAMETER Agente
  Usa o motor antigo (LLM com ferramentas), para comparar a mesma conversa.

.PARAMETER Sombra
  Usa o modo sombra: responde o motor antigo e o /turno mostra o que o motor novo entendeu (passo extrator_sombra).

.PARAMETER Roteiro
  Roda um roteiro sem interacao (ex.: stress/a07_tudo_junto, loop_passageiros) com veredito no fim.

.PARAMETER Modelo
  Outro modelo da NVIDIA so nesta execucao (padrao nvidia/nemotron-3.5-lightning-30b-a3b).

.PARAMETER Raciocinio
  Liga o raciocinio do modelo (mais lento).

.EXAMPLE
  .\testar-comandos.ps1
  Conversa interativa com o motor por comandos.

.EXAMPLE
  .\testar-comandos.ps1 -Roteiro stress/a07_tudo_junto
  Reproduz um roteiro e mostra OK/FALHA.

.EXAMPLE
  .\testar-comandos.ps1 -Agente
  A mesma conversa, mas com o motor antigo.

.EXAMPLE
  .\testar-comandos.ps1 -Rapido
  Bateria rapida (17 roteiros, ~15-20 min) com placar SUCESSO x/17 no fim.
#>
[CmdletBinding()]
param(
    [switch]$Rapido,
    [switch]$Agente,
    [switch]$Sombra,
    [string]$Roteiro,
    [string]$Modelo = 'nvidia/nemotron-3.5-lightning-30b-a3b',
    [switch]$Raciocinio,
    [switch]$Lista
)

$ErrorActionPreference = 'Stop'
$raiz = $PSScriptRoot
$envArq = Join-Path $raiz 'apps\api\.env.atendimento-local'

if ($Rapido) { $Roteiro = 'rapido' }
$motor = 'comandos'
if ($Agente) { $motor = 'agente' }
if ($Sombra) { $motor = 'sombra' }

# O motor por comandos (e o sombra) dependem do roteador Jev.
if (-not $Lista -and $motor -ne 'agente' -and (Test-Path -LiteralPath $envArq)) {
    $temJev = Select-String -LiteralPath $envArq -Pattern '^\s*TYPESAFE_API_KEY\s*=\s*\S' -Quiet
    if (-not $temJev -and -not $env:TYPESAFE_API_KEY) {
        Write-Host "Falta a TYPESAFE_API_KEY (Jev): sem ela o motor '$motor' cai no motor antigo." -ForegroundColor Yellow
        Write-Host "Rode .\testar-atendimento.ps1 -NovasChaves e cole a chave do Jev quando pedir." -ForegroundColor Yellow
        exit 1
    }
}

# Variaveis so desta execucao (o ambiente do SO tem prioridade sobre o arquivo de credenciais).
$vars = [ordered]@{
    ATENDIMENTO_V2_MOTOR = $motor
    ATENDIMENTO_V2_JUIZ  = 'jev'
    LLM_PROVEDOR         = 'nvidia'
    ATENDIMENTO_V2_MODELO = $Modelo
    LLM_MODELO_RESERVA   = 'off'
    LLM_HEDGE_MS         = '0'
    LLM_SEM_RACIOCINIO   = $(if ($Raciocinio) { 'false' } else { 'true' })
    ATD_ORCAMENTO_S      = '60'
    LLM_TIMEOUT_S        = '45'
}
$anteriores = @{}
foreach ($k in $vars.Keys) {
    $anteriores[$k] = [Environment]::GetEnvironmentVariable($k, 'Process')
    [Environment]::SetEnvironmentVariable($k, $vars[$k], 'Process')
}

Write-Host "Motor: $motor · modelo: $Modelo · raciocinio: $(if ($Raciocinio) { 'ligado' } else { 'desligado' })" -ForegroundColor Cyan
if (-not $Roteiro -and -not $Lista) {
    Write-Host "Digite como o cliente. /turno mostra o que foi entendido; /estado a compra; /reset recomeca; /sair encerra." -ForegroundColor DarkGray
}

$params = @{}
if ($Roteiro) { $params['Roteiro'] = $Roteiro }
if ($Lista) { $params['Lista'] = $true }

$codigo = 0
try {
    & (Join-Path $raiz 'testar-atendimento.ps1') @params
    $codigo = $LASTEXITCODE
} finally {
    foreach ($k in $vars.Keys) { [Environment]::SetEnvironmentVariable($k, $anteriores[$k], 'Process') }
}
exit $codigo
