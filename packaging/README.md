# Empacotamento MSIX (Microsoft Store)

Artefatos e passos para gerar a build **store** do Controlinho como pacote **MSIX
full-trust**. Veja também a [listagem da Store](STORE-LISTING.md) e a [política de privacidade](../PRIVACY.md).

## O que muda na build `store`

A mesma base de código gera dois alvos via **build tags** (`-tags store`):

| | Desktop `.exe` (padrão) | MSIX `store` |
|---|---|---|
| Firewall automático (`netsh`) | sim | **não** (prompt nativo do Windows) |
| Auto-elevação UAC | sim | **não** (roda como usuário padrão) |
| Auto-início | `schtasks /sc onlogon` | **`windows.startupTask`** (WinRT, toggle na bandeja) |
| Flags `-install`/`-uninstall`/`-setupfw` | sim | **não** (a Store instala/desinstala) |
| PIN de pareamento | sim | sim (compartilhado) |

Arquivos por tag: `install_windows.go` (`windows && !store`), `install_store.go`
(`windows && store`, StartupTask via WinRT), `installcli_desktop.go` (`!store`),
`installcli_store.go` (`store`).

## Pré-requisitos

- Go (toolchain do projeto).
- **Windows 10/11 SDK** — fornece `makeappx.exe` e `signtool.exe`.
- (Submissão) Conta no **Partner Center** (~US$ 19), nome do app reservado, e uma
  **política de privacidade** publicada numa URL.

## Gerar os assets dos ícones

Os logos da Store são gerados pelo mesmo `gen_icons.go`, em `packaging/Assets/`:

```bash
go run gen_icons.go
```

Produz `StoreLogo.png` (50×50), `Square44x44Logo.png`, `Square150x150Logo.png` e
`Wide310x150Logo.png`. (Variantes de escala como `*.scale-200.png` são opcionais;
os nomes-base bastam para um pacote válido.)

## Build do pacote

```powershell
# Pacote não-assinado, pronto pra upload no Partner Center:
.\packaging\build-msix.ps1

# Pacote assinado para teste local (sideload). O Subject do cert TEM que ser igual
# ao Identity/Publisher do Package.appxmanifest:
.\packaging\build-msix.ps1 -Sign -PfxPath .\teste.pfx -PfxPassword (Read-Host -AsSecureString)
```

O script: compila `go build -tags store -ldflags "-s -w -H windowsgui"`, monta
`pkg\` (exe + manifesto + `Assets\`), e roda `makeappx pack`. **Para a Store não
assine** — o Partner Center assina na ingestão; a assinatura é só pro sideload.

> Antes de submeter, preencha no `Package.appxmanifest` os campos que vêm do Partner
> Center: `Identity/Name`, `Identity/Publisher` (`CN=<GUID>`) e `PublisherDisplayName`.

## Teste local (jeito rápido, sem certificado)

Para testar o pacote — em especial o **StartupTask** — sem gerar/assinar certificado,
use o registro em **Modo de Desenvolvedor** (Configurações → Privacidade e segurança →
Para desenvolvedores):

```powershell
.\packaging\build-msix-local.ps1        # build -tags store + Add-AppxPackage -Register + abre o app
```

Isso dá ao app **identidade de pacote** (necessária pro WinRT StartupTask). Para
verificar o auto-início **sem clicar na bandeja**:

```powershell
.\packaging\verify-startuptask.ps1            # mostra o estado atual
.\packaging\verify-startuptask.ps1 -Enable    # liga
.\packaging\verify-startuptask.ps1 -Disable   # desliga
```

Esse helper roda a mesma API WinRT que o [install_store.go](../install_store.go) usa,
sob a identidade do pacote (via `Invoke-CommandInDesktopPackage`). **Verificado:**
`Disabled → Enabled → Disabled`. Remover o pacote de teste:
`Get-AppxPackage *Controlinho* | Remove-AppxPackage`.

## Instalar localmente a partir do `.msix` assinado (sideload)

```powershell
Add-AppxPackage -Path .\Controlinho.msix   # requer o cert de teste confiado na maquina
```

## ✅ Checklist de verificação (antes de submeter)

- [ ] App instala e abre; **bandeja** aparece com ícone.
- [ ] Servidores `8080`/`8443` sobem; **loopback funciona** (probe de instância única
      em `127.0.0.1/info` e auto-open do navegador).
- [ ] Na primeira escuta da porta, aparece o **prompt do Firewall do Windows** (não há
      mais abertura silenciosa via `netsh`).
- [ ] Celular conecta na LAN, **instala a CA**, instala o PWA, controla mouse/teclado.
- [ ] **PIN/pareamento**: escanear o QR verde conecta sem digitar nada; digitar o IP na
      mão exige o **PIN** mostrado na página `/qr`; PIN errado → não conecta.
- [ ] **CA + token persistem** após simular um update (instalar versão N+1 por cima):
      o celular **não** reinstala o certificado nem re-pareia.
- [x] ✅ **StartupTask (Opção A) — VERIFICADO** com `verify-startuptask.ps1`
      (`Disabled → Enabled → Disabled`) e via `-EncodedCommand` (o caminho exato do
      `install_store.go`), tudo sob identidade de pacote. **Só funciona com o app
      rodando a partir do MSIX instalado/registrado** (precisa de *package identity*);
      o `-tags store` como `.exe` solto reporta "sem pacote" e o toggle não faz nada.
      Na UI: alterne "Iniciar com o Windows" na bandeja e confira em *Gerenciador de
      Tarefas → Inicializar*.

> **Detalhe técnico que o teste on-device revelou:** no **Windows PowerShell 5.1** o
> tipo `[System.WindowsRuntimeSystemExtensions]` (que fornece o `AsTask` usado para
> aguardar o `IAsyncOperation` do WinRT) **não resolve** sem antes carregar a assembly
> `System.Runtime.WindowsRuntime`. O `install_store.go` já faz isso no preâmbulo do
> script (`[void][...WindowsRuntimeMarshal]` + `Add-Type`). Sem essa carga, dá
> "não foi possível localizar o tipo".

### Se o StartupTask via WinRT der problema

`install_store.go` dirige a API WinRT `StartupTask` via PowerShell (sem CGO, sem
dependência nova) — **comprovadamente funcional**. Se ainda assim quiser simplificar,
o fallback para a **Opção B** é trivial: fazer `autoStartEnabled()` retornar `false` e
`setAutoStart()` retornar `nil` (no-ops) e remover o item da bandeja em
`tray_windows.go`. O usuário então liga o início automático em *Configurações → Apps →
Inicialização* (o manifesto já declara a `windows.startupTask`).

## Submissão (Partner Center)

- Listagem: descrição, **screenshots**, classificação etária.
- **Justificativa de `runFullTrust`** (capability restrita): *"controle remoto do
  desktop a partir do celular na rede local; requer injeção de input via Win32,
  incompatível com sandbox."*
- URL da política de privacidade. Documente que o app instala uma **CA raiz no
  dispositivo do próprio usuário, por escolha dele**.

## ⚠️ O furo que a Store NÃO resolve

A Store remove o aviso do SmartScreen **no PC**, mas **não faz nada** pelo passo que
mais trava: instalar a **CA no celular**. Esse atrito continua idêntico. Ver §9 do
plano.
