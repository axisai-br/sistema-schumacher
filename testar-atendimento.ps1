<#
.SYNOPSIS
  Testa conversas do atendimento v2 localmente (agente real + LLM real, dados de teste em memoria).

.DESCRIPTION
  Sem WhatsApp, sem banco de dados e sem o resto do sistema. Na primeira execucao pede a chave
  da NVIDIA (NVIDIA_API_KEY, obtida em build.nvidia.com) e grava apps/api/.env.atendimento-local
  (arquivo ignorado pelo git) a partir de apps/api/.env.atendimento-local.example. As chaves
  nunca sao exibidas. Depois roda "go run ./cmd/atendimento-local" dentro de apps/api.

.PARAMETER NovasChaves
  Pede as chaves de novo e regrava o arquivo de credenciais.

.PARAMETER Roteiro
  Executa um roteiro sem interacao. Nome de roteiro pronto (ex.: loop_passageiros) ou caminho de arquivo.

.PARAMETER Caso
  Roda caso(s) de avaliacao com cliente simulado por LLM: um nome ou "todos".

.PARAMETER Modelo
  Sobrescreve ATENDIMENTO_V2_MODELO apenas nesta execucao.

.PARAMETER Lista
  Lista os casos e roteiros disponiveis (nao precisa de chave).

.EXAMPLE
  .\testar-atendimento.ps1
  Abre a conversa interativa (prompt "voce> ").

.EXAMPLE
  .\testar-atendimento.ps1 -Roteiro loop_passageiros
  Reproduz uma conversa real de producao e mostra como o bot responde.

.EXAMPLE
  .\testar-atendimento.ps1 -Caso todos
  Roda todos os casos de avaliacao.

.EXAMPLE
  .\testar-atendimento.ps1 -Modelo meta/llama-3.3-70b-instruct
  Conversa interativa com outro modelo, so nesta execucao.

.EXAMPLE
  .\testar-atendimento.ps1 -NovasChaves
  Troca a chave salva.
#>
[CmdletBinding()]
param(
    [switch]$NovasChaves,
    [string]$Roteiro,
    [string]$Caso,
    [string]$Modelo,
    [switch]$Lista
)

$ErrorActionPreference = 'Stop'

# Console em UTF-8 (acentos corretos).
try {
    chcp 65001 > $null
    [Console]::InputEncoding = [System.Text.Encoding]::UTF8
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch { }

$raiz = $PSScriptRoot
$api = Join-Path $raiz 'apps\api'
$envArq = Join-Path $api '.env.atendimento-local'
$envExemplo = Join-Path $api '.env.atendimento-local.example'

if (-not (Test-Path -LiteralPath $api)) {
    Write-Host "Nao encontrei apps\api em $raiz. Rode este script a partir da raiz do repositorio." -ForegroundColor Red
    exit 1
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "O Go nao foi encontrado no PATH." -ForegroundColor Red
    Write-Host "Instale em https://go.dev/dl/ (versao 1.22 ou superior), feche e abra o terminal e tente de novo."
    exit 1
}

function Ler-Segredo([string]$pergunta) {
    $sec = Read-Host -Prompt $pergunta -AsSecureString
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec)
    try { return ([Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)).Trim() }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr) }
}

function Mascarar([string]$chave, [string]$prefixo) {
    if ($chave.Length -le 4) { return "$prefixo…" }
    return "$prefixo…" + $chave.Substring($chave.Length - 4)
}

function Gravar-Credenciais {
    if (-not (Test-Path -LiteralPath $envExemplo)) {
        Write-Host "Modelo $envExemplo nao encontrado." -ForegroundColor Red
        exit 1
    }
    Write-Host ""
    Write-Host "Configuracao das chaves (nada do que voce colar aparece na tela)." -ForegroundColor Cyan

    $nvidia = ''
    while ($true) {
        $nvidia = Ler-Segredo 'Cole sua NVIDIA_API_KEY (build.nvidia.com)'
        if ($nvidia -eq '') {
            Write-Host 'A chave nao pode ficar vazia.' -ForegroundColor Yellow
            continue
        }
        if (-not $nvidia.StartsWith('nvapi-')) {
            Write-Host 'Aviso: chaves da NVIDIA normalmente comecam com "nvapi-".' -ForegroundColor Yellow
            $r = Read-Host 'Continuar mesmo assim? (s/N)'
            if ($r -notmatch '^[sSyY]') { continue }
        }
        break
    }
    $openai = Ler-Segredo 'OPENAI_API_KEY (opcional, so para transcrever audio - Enter para pular)'

    $linhas = Get-Content -LiteralPath $envExemplo -Encoding UTF8
    $saida = foreach ($l in $linhas) {
        if ($l -match '^NVIDIA_API_KEY=') { "NVIDIA_API_KEY=$nvidia" }
        elseif ($l -match '^OPENAI_API_KEY=' -and $openai -ne '') { "OPENAI_API_KEY=$openai" }
        else { $l }
    }
    $texto = ($saida -join "`r`n") + "`r`n"
    [System.IO.File]::WriteAllText($envArq, $texto, (New-Object System.Text.UTF8Encoding($false)))

    Write-Host "Salvo em apps\api\.env.atendimento-local (ignorado pelo git)." -ForegroundColor Green
    Write-Host ("  NVIDIA_API_KEY = " + (Mascarar $nvidia 'nvapi-'))
    if ($openai -ne '') { Write-Host ("  OPENAI_API_KEY = " + (Mascarar $openai 'sk-')) }
    Write-Host ""
}

if (-not $Lista) {
    if ($NovasChaves -or -not (Test-Path -LiteralPath $envArq)) {
        Gravar-Credenciais
    }
}

# Argumentos do simulador.
$args2 = @()
if ($Lista) { $args2 += '-lista' }
elseif ($Caso) { $args2 += @('-caso', $Caso) }
elseif ($Roteiro) {
    $arqRot = Join-Path $api ("cmd\atendimento-local\roteiros\" + ($Roteiro -replace '\.txt$', '') + '.txt')
    if (Test-Path -LiteralPath $arqRot) { $Roteiro = $arqRot }
    $args2 += @('-roteiro', $Roteiro)
}

$modeloAnterior = $env:ATENDIMENTO_V2_MODELO
if ($Modelo) { $env:ATENDIMENTO_V2_MODELO = $Modelo }

$codigo = 0
Push-Location $api
try {
    & go run ./cmd/atendimento-local @args2
    $codigo = $LASTEXITCODE
} finally {
    Pop-Location
    if ($Modelo) { $env:ATENDIMENTO_V2_MODELO = $modeloAnterior }
}
exit $codigo
