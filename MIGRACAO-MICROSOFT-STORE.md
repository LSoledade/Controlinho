# 🏪 Migração para a Microsoft Store

> Plano de trabalho para publicar o **PC Remote** na Microsoft Store como pacote **MSIX full-trust**.
> Base: v2.2.0. Status: **Fase 1 (código) e Fase 2 (empacotamento) implementadas**; faltam as contas/decisões
> manuais da Fase 0 e a submissão (Fase 3). Decisões tomadas: distribuição **dupla** (`.exe` + MSIX via
> build tags), auto-início **Opção A** (StartupTask via WinRT), **PIN** de pareamento **sim**.
> Empacotamento em [`packaging/`](packaging/README.md). ✅ O StartupTask via WinRT foi **verificado
> on-device** (registro em Modo de Desenvolvedor + identidade de pacote): `Disabled → Enabled →
> Disabled`. Falta apenas o que é manual/conta (Partner Center, política de privacidade, submissão).

---

## 1. Objetivo e escopo

**Objetivo:** distribuir o app pela Microsoft Store para que amigos instalem **sem o aviso do SmartScreen**, com **assinatura da Microsoft** e **auto-update** — sem comprar certificado de código.

**Fora de escopo:** o passo de instalar a CA local **no celular** (Android/iOS). A Store **não resolve isso** — continua igual. Ver [§9 Furo honesto](#9-o-furo-honesto-que-a-store-não-resolve).

**Custo:** conta de desenvolvedor individual no Partner Center — **~US$ 19, pagamento único**.

---

## 2. Resumo executivo (TL;DR)

| Item | Situação |
|---|---|
| Miolo do app (servidores, WS, input, TLS, QR, bandeja, PWA) | ✅ **Não muda** |
| Ações de energia (shutdown/sleep/monitor) | ✅ Funcionam como usuário padrão |
| Injeção de teclado/mouse | ✅ OK em MSIX **full-trust** (não é sandbox) |
| Firewall automático (`netsh`) | ❌ **Remover** — usar prompt nativo do Windows |
| Auto-elevação UAC (`runas`) | ❌ **Remover** — nada mais precisa de admin |
| Auto-início via `schtasks` | 🔄 **Trocar** por `StartupTask` do MSIX |
| Flags `-install`/`-uninstall`/`-setupfw` + `install.bat` | ❌ **Aposentar** — a Store instala/desinstala |
| Empacotamento MSIX + manifesto + assets | ➕ **Trabalho novo** |
| PIN de pareamento (segurança) | ➕ **Recomendado** (ajuda a passar na certificação) |

**Esforço de código real é pequeno** — o que viola a política da Store está concentrado em [install_windows.go](install_windows.go) e no toggle da bandeja.

---

## 3. Pré-requisitos e decisões a tomar antes de começar

- [ ] **Conta Partner Center** criada (~US$ 19) e nome de publisher reservado.
- [ ] **Reservar o nome do app** na Store (define o `Identity/Name` e `Publisher` do manifesto).
- [x] **Decisão sobre o auto-início** → **Opção A** (botão na bandeja via WinRT `StartupTask`). Implementado em [install_store.go](install_store.go).
- [x] **Decisão sobre o PIN de pareamento** → **Sim**. Implementado em [token.go](token.go) + cliente.
- [ ] **Política de privacidade** publicada numa URL (obrigatório na submissão).
- [x] **Manter a distribuição via `.exe` em paralelo?** → **Sim** (distribuição dupla via build tags `store`).

---

## 4. O que NÃO muda

Nenhuma alteração necessária em:

- `main.go` — servidores HTTP/HTTPS, WebSocket, dispatch, allowlist, `checkOrigin` (exceto a remoção das flags de install e a adição opcional do PIN).
- `tlsx.go` — CA local + emissão dinâmica. `dataDir()` usa `os.UserCacheDir()` → sob MSIX é **redirecionado pro armazenamento do pacote e persiste entre updates**. ✅ A CA não se perde ao atualizar → amigo **não reinstala o certificado no celular** a cada update.
- `input_windows.go` — injeção via `user32.dll` funciona em full-trust.
- `qr.go` — página de QR e auto-open.
- `client/` — PWA, manifest, service worker, ícones.
- `tray_windows.go` — **exceto** o toggle de auto-início (ver §5.4).

---

## 5. Refatoração do app (Fase 1)

### 5.1 Remover o firewall automático

**Arquivos:** [install_windows.go](install_windows.go), [main.go](main.go)

Remover: `addFirewallRule`, `ensureFirewall`, `shellExecuteRunAs`, a flag `-setupfw` e o bloco `if *setupFW` em `main()`.

**Tradeoff:** sem abertura silenciosa da porta. Na primeira vez que o socket abre, aparece o **prompt nativo do Windows Firewall** ("Permitir acesso"). É um clique do usuário; "Permitir em rede pública" pode pedir admin uma vez.

### 5.2 Remover a auto-elevação UAC

**Arquivo:** [install_windows.go](install_windows.go)

Some junto com o firewall (`shellExecuteRunAs`, verbo `runas`, DLL `shell32`/`ShellExecuteW`).

**Tradeoff:** nenhum. A elevação só existia para o firewall. Sem firewall, **nada no app precisa de admin** — e a política da Store exige rodar como usuário padrão.

### 5.3 Auto-início: `schtasks` → `StartupTask`

**Arquivos:** [install_windows.go](install_windows.go) (`installSelf`, `uninstallSelf`, `taskInstalled`), manifesto MSIX.

O `schtasks /create /sc onlogon` é **proibido** para apps empacotados. Auto-início passa a ser a extensão **`windows.startupTask`** declarada no manifesto (ver [§6](#6-manifesto-msix-exemplo)).

- **Opção A (com botão):** chamar a WinRT `StartupTask.RequestEnableAsync()` / `Disable()` do Go.
  - Bindings: [`github.com/saltosystems/winrt-go`](https://github.com/saltosystems/winrt-go), **ou** um helper mínimo em C/C++, **ou** um pequeno `.ps1`/utilitário empacotado.
  - `taskInstalled()` vira uma consulta a `StartupTask.GetForCurrentPackageAsync().State`.
- **Opção B (sem botão):** remover o item da bandeja; usuário liga em *Configurações → Apps → Inicialização*. Zero código.

**Tradeoff:** (a) **não dá mais para forçar** o auto-início — o usuário (ou política do Windows) decide; (b) a Opção A é o **único ponto de trabalho técnico não-trivial** desta migração (WinRT a partir do Go).

### 5.4 Toggle da bandeja

**Arquivo:** [tray_windows.go:35-60](tray_windows.go#L35-L60)

O item `mAuto` ("Iniciar com o Windows") hoje chama `installSelf`/`uninstallSelf`. Passa a chamar a API `StartupTask` (Opção A) ou é removido (Opção B). `taskInstalled()` no estado inicial do checkbox vira a consulta de estado da StartupTask.

### 5.5 Limpar flags e scripts de install

**Arquivos:** [main.go](main.go), [install.bat](install.bat)

Remover as flags `-install`, `-uninstall`, `-setupfw` e seus blocos em `main()`. Apagar `install.bat`. Quem instala/desinstala é a Store.

**Tradeoff:** nenhum — é simplificação.

---

## 6. Manifesto MSIX (exemplo)

`Package.appxmanifest` — app **full-trust** (`runFullTrust`) com **StartupTask**:

```xml
<?xml version="1.0" encoding="utf-8"?>
<Package
  xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
  xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
  xmlns:uap5="http://schemas.microsoft.com/appx/manifest/uap/windows10/5"
  xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"
  IgnorableNamespaces="uap uap5 rescap">

  <!-- Name e Publisher vêm do Partner Center após reservar o nome -->
  <Identity Name="SeuPublisher.PCRemote"
            Publisher="CN=XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"
            Version="2.2.0.0"
            ProcessorArchitecture="x64" />

  <Properties>
    <DisplayName>PC Remote</DisplayName>
    <PublisherDisplayName>Seu Nome</PublisherDisplayName>
    <Logo>Assets\StoreLogo.png</Logo>
  </Properties>

  <Dependencies>
    <TargetDeviceFamily Name="Windows.Desktop"
                        MinVersion="10.0.17763.0"
                        MaxVersionTested="10.0.22621.0" />
  </Dependencies>

  <Resources>
    <Resource Language="pt-BR" />
  </Resources>

  <Applications>
    <Application Id="PCRemote"
                 Executable="pc-remote.exe"
                 EntryPoint="Windows.FullTrustApplication">
      <uap:VisualElements
        DisplayName="PC Remote"
        Description="Controle o PC pelo celular na rede local"
        BackgroundColor="transparent"
        Square150x150Logo="Assets\Square150x150Logo.png"
        Square44x44Logo="Assets\Square44x44Logo.png" />
      <Extensions>
        <!-- Auto-início: substitui o schtasks /sc onlogon -->
        <uap5:Extension Category="windows.startupTask"
                        Executable="pc-remote.exe"
                        EntryPoint="Windows.FullTrustApplication">
          <uap5:StartupTask TaskId="PCRemoteStartup"
                            Enabled="false"
                            DisplayName="PC Remote" />
        </uap5:Extension>
      </Extensions>
    </Application>
  </Applications>

  <Capabilities>
    <rescap:Capability Name="runFullTrust" />          <!-- exige justificativa na submissão -->
    <Capability Name="privateNetworkClientServer" />   <!-- servir/ouvir na LAN -->
    <Capability Name="internetClientServer" />
  </Capabilities>
</Package>
```

> ⚠️ `runFullTrust` é uma **capability restrita** — na submissão a Microsoft pede **justificativa**. Use: *"controle remoto do desktop a partir do celular na rede local; requer injeção de input via Win32, incompatível com sandbox."*

**Assets de imagem necessários** (gerar a partir de `client/icon-512.png` com [gen_icons.go](gen_icons.go), estendendo-o):
`StoreLogo.png` (50×50), `Square150x150Logo.png`, `Square44x44Logo.png`, e (opcional) `Wide310x150Logo.png`. Mais os **screenshots** da listagem (1366×768 ou similar).

---

## 7. Empacotamento e build (Fase 2)

Pipeline de exemplo (Windows SDK fornece `makeappx` e `signtool`):

```bat
:: 1. Build do binário (sem console; a bandeja é a UI)
go build -ldflags "-s -w -H windowsgui" -o pkg\pc-remote.exe .

:: 2. Montar a pasta do pacote
::    pkg\
::    ├── pc-remote.exe
::    ├── Package.appxmanifest
::    └── Assets\  (logos)

:: 3. Empacotar
makeappx pack /d pkg /p PCRemote.msix

:: 4. (Só para testar localmente / sideload) assinar com cert self-signed:
signtool sign /fd SHA256 /a /f teste.pfx PCRemote.msix
::    -> para a Store NÃO precisa assinar: o Partner Center assina na ingestão.
```

**Verificações pós-empacotamento (sideload local antes de submeter):**
- [ ] App instala e abre; bandeja aparece com ícone.
- [ ] Servidores 8080/8443 sobem; **loopback funciona** (full-trust roda fora do AppContainer — o probe de instância única em `127.0.0.1/info` e o auto-open do navegador devem funcionar).
- [ ] Celular conecta na LAN, instala a CA, instala o PWA, controla o mouse/teclado.
- [ ] CA persiste após simular um update (instalar versão N+1 por cima).
- [ ] Toggle de auto-início (Opção A) liga/desliga a StartupTask.

---

## 8. Segurança: PIN de pareamento (recomendado)

**Por quê:** hoje o modelo é "confie na topologia da rede" — **sem autenticação**. Um revisor da Store avaliando *controle remoto do PC sem senha* pode **reprovar**. Adicionar um PIN (a) melhora a chance de aprovação e (b) conserta o ponto mais fraco do modelo de segurança.

**Design sugerido (mínimo):**
- Servidor gera um **token de sessão** na inicialização (em memória).
- [qr.go](qr.go) embute o token na URL do **QR verde** (app seguro).
- [main.go](main.go) `handleWS` (e/ou um cookie no primeiro acesso HTTPS) exige o token; sem token válido → `403`.
- [client/index.html](client/index.html) guarda o token no `localStorage` e o envia ao abrir o WebSocket.

**Tradeoff:** quem digitar o IP na mão (sem escanear o QR) precisa do PIN. Pequeno atrito, ganho grande de segurança. **Fazer isto independe da decisão da Store.**

---

## 9. O furo honesto que a Store NÃO resolve

A Store remove o aviso do `.exe` **no PC** — que é um "Executar assim mesmo" de **uma vez só**. Ela **não faz nada** pela parte que mais trava os amigos: **instalar a CA no celular** (o passo confuso de "Certificado CA" no Android). Esse atrito **continua idêntico**.

**Pese isto:** você faz o retrabalho para eliminar a fricção *menor* (PC), e a *maior* (celular) permanece. Se o incômodo real dos seus amigos é o certificado no telefone, a Store não é a alavanca certa.

---

## 10. Riscos de certificação e mitigações

| Risco | Mitigação |
|---|---|
| Reprovação por "controle remoto sem autenticação" | PIN de pareamento ([§8](#8-segurança-pin-de-pareamento-recomendado)) |
| `runFullTrust` questionado | Justificativa clara na submissão (injeção de input) |
| App instala CA raiz em dispositivos | É no **dispositivo do próprio usuário**, por escolha dele; documentar na descrição/privacidade |
| Modificação de firewall sinalizada | Já removida (§5.1) — usa o prompt nativo |
| Exige elevação | Já removida (§5.2) — roda como usuário padrão |

---

## 11. Estratégia de distribuição dupla

Recomendado **manter a build `.exe`** em paralelo (GitHub Releases) além da Store:
- cobre quem não quer/usa a Store;
- útil em redes corporativas que bloqueiam a Store;
- custo de manutenção baixo (o mesmo binário; só o empacotamento difere).

Isolar o código específico de Store/desktop por **build tags** ou pasta de packaging, para um único `go build` servir aos dois alvos.

---

## 12. Checklist consolidado por fase

> Nota: com **distribuição dupla**, o que viola a política da Store é **isolado por
> build tag** (`-tags store`), não removido — a build `.exe` mantém firewall/UAC/schtasks.

**Fase 0 — Contas e decisões**
- [ ] Conta Partner Center (~US$ 19)
- [ ] Reservar nome do app / publisher
- [x] Decidir Opção A vs B do auto-início → **A**
- [x] Decidir PIN → **sim**
- [ ] Publicar política de privacidade (URL)

**Fase 1 — Refatorar o app** (✅ implementada — isolada por build tag)
- [x] Firewall automático fora da build `store` (§5.1) — só em [install_windows.go](install_windows.go) (`windows && !store`)
- [x] Auto-elevação UAC fora da build `store` (§5.2) — idem
- [x] `schtasks` → `StartupTask` na build `store` (§5.3) — [install_store.go](install_store.go) (WinRT)
- [x] Toggle da bandeja unificado (§5.4) — `autoStartEnabled`/`setAutoStart` em [tray_windows.go](tray_windows.go)
- [x] Flags `-install`/`-uninstall`/`-setupfw` fora da build `store` (§5.5) — [installcli_store.go](installcli_store.go) / [installcli_desktop.go](installcli_desktop.go); `install.bat` mantido para a build `.exe`
- [x] PIN de pareamento (§8) — [token.go](token.go), [main.go](main.go), [qr.go](qr.go), [client/index.html](client/index.html)

**Fase 2 — Empacotar** (✅ feita e validada localmente)
- [x] `gen_icons.go` estendido para os assets da Store ([gen_icons.go](gen_icons.go) → `packaging/Assets/`)
- [x] `Package.appxmanifest` escrito ([packaging/Package.appxmanifest](packaging/Package.appxmanifest))
- [x] Pipeline `go build -tags store` → `makeappx` ([packaging/build-msix.ps1](packaging/build-msix.ps1)) — **`PCRemote.msix` gerado (4,3 MB)**
- [x] Teste local via Modo de Desenvolvedor ([packaging/build-msix-local.ps1](packaging/build-msix-local.ps1)): app instala, sobe `:8080`/`:8443`, serve `/info`, token OK
- [x] **StartupTask verificado** ([packaging/verify-startuptask.ps1](packaging/verify-startuptask.ps1)): `Disabled → Enabled → Disabled` sob identidade de pacote

**Fase 3 — Submeter**
- [ ] Listagem (descrição, screenshots, classificação etária)
- [ ] Justificativa de `runFullTrust`
- [ ] Upload no Partner Center e submeter à certificação

**Fase 4 — Pós-publicação**
- [ ] Validar auto-update com uma versão N+1
- [ ] Manter build `.exe` paralela (§11)

---

## 13. Veredito

Viável, e o código coopera — o "sujo" está isolado. O **único trabalho técnico não-trivial** é o `StartupTask` via WinRT (Opção A); todo o resto é remoção de código + empacotamento. O **PIN de pareamento** vale por si só. **Mas** lembre do [§9](#9-o-furo-honesto-que-a-store-não-resolve): a Store resolve o atrito menor, e o maior (CA no celular) continua.
