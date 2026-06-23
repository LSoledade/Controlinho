# 🖥️ Controlinho

> **v2.2.0** · disponível na **Microsoft Store** (e como `.exe` standalone)

Controle o PC Windows pelo celular, na rede local. Servidor Go + cliente PWA num único binário, sem nuvem, sem dependências externas, sem instalação extra.

- **Trackpad** — mover, clique esquerdo/direito/meio, **arrastar**, scroll com 2 dedos, sensibilidade ajustável
- **Mídia & Volume** — play/pause, próxima, anterior, volume +/−, mudo, voltar/avançar navegador
- **Teclado / Atalhos** — digitar texto (Unicode), Ctrl+C/V/Z…, Alt+F4, Win+L, **setas e navegação (D-pad)**
- **Energia** — desligar monitor, suspender, reiniciar, desligar (com confirmação)

Tudo num executável único que sobe o servidor **e** serve a interface, com **HTTPS confiável** para instalar como app no celular.

---

## Instalar no PC

- **Microsoft Store (recomendado):** [apps.microsoft.com/detail/9NWT7X0QSGBJ](https://apps.microsoft.com/detail/9NWT7X0QSGBJ) — instala sem o aviso do SmartScreen e com **auto-update**. *(Link ativo após a publicação; ID da Store: `9NWT7X0QSGBJ`.)*
- **Standalone (`.exe`):** compile do código (veja [Build](#build)) — útil sem a Store ou em redes corporativas que a bloqueiam.

As duas distribuições saem do **mesmo código** (build tag `store`); só mudam o empacotamento e o mecanismo de auto-início — o miolo (servidores, WebSocket, input, TLS, PIN, PWA) é idêntico.

---

## Como funciona

```
┌────────────┐   WebSocket (JSON) sobre TLS   ┌──────────────┐
│  Celular   │ ◄───────────────────────────► │  PC Windows  │
│  (PWA)     │   rede local / Tailscale      │  (Go server  │
│            │                               │  + user32)   │
└────────────┘                               └──────────────┘
```

O servidor sobe **duas** portas:

| Porta  | Protocolo | Para quê |
|--------|-----------|----------|
| `8080` | HTTP      | Setup inicial: baixar o certificado e abrir a versão segura |
| `8443` | HTTPS     | O app de verdade (PWA instalável + WebSocket seguro `wss://`) |

As ações (mouse, teclado, energia) são executadas direto na API do Windows via `user32.dll`.

---

## Por que HTTPS? (e por que isso é necessário no Android)

Para o Chrome no Android oferecer **"Instalar app"** e registrar o service worker, a página precisa estar num **contexto seguro** (HTTPS confiável). A exceção `localhost` **não** vale para um IP de LAN tipo `192.168.x.x` — sobre `http://` o Android não instala como app de verdade.

A solução, sem depender de nuvem ou serviço de terceiros, é a mesma técnica do [`mkcert`](https://github.com/FiloSottile/mkcert):

1. Na **primeira execução**, o servidor gera uma **autoridade certificadora (CA) local** própria (`pc-remote-ca.crt` / `.key`, guardados num diretório por usuário e local da máquina — `%LocalAppData%\pc-remote` no Windows).
2. Ele emite um certificado HTTPS para os IPs atuais do PC (LAN + Tailscale), assinado por essa CA, e o **reemite sob demanda** quando o IP muda ou o certificado se aproxima do vencimento.
3. Você instala a CA no celular **uma única vez**. A partir daí o Chrome confia em tudo que o servidor assina → cadeado verde, contexto seguro, service worker e instalação do PWA funcionando.

A chave privada da CA **nunca sai do PC**.

> 💡 Se você já usava uma versão anterior (com a CA ao lado do `.exe`), ela é **migrada automaticamente** para o novo diretório na primeira execução desta versão — **não** é preciso reinstalar o certificado no celular após atualizar.
>
> ⚠️ **Não compartilhe o `pc-remote-ca.key`.** Quem tiver esse arquivo pode emitir certificados confiáveis para o seu celular. Ele já fica fora do controle de versão (veja `.gitignore`).

---

## Build

Requisitos: **Go 1.21+** (testado com Go 1.25). Windows/amd64. Não precisa de GCC.

```bat
:: Versão de release (binário enxuto)
go build -ldflags "-s -w" -o pc-remote.exe .

:: Sem janela de console (inicia silencioso pelo Task Scheduler)
go build -ldflags "-s -w -H windowsgui" -o pc-remote.exe .

:: Build MSIX (Microsoft Store) — mesmo código, build tag `store`
go build -tags store -ldflags "-s -w -H windowsgui" -o pkg/pc-remote.exe .
```

O cliente é embutido no binário via `//go:embed`, então qualquer mudança em `client/` exige rebuild. O empacotamento MSIX completo (manifesto, assets, `makeappx`, teste local e verificação do StartupTask) está em [`packaging/`](packaging/README.md).

---

## Conectar pelo QR Code (jeito mais fácil)

Quando você roda o `pc-remote.exe` **com console** (run manual), ele:

- abre automaticamente a **página de conexão** no navegador do PC (`http://127.0.0.1:8080/qr`), e
- desenha um **QR Code no próprio terminal**.

A página mostra um **card por rede detectada** (Wi-Fi local e, se houver, Tailscale), cada um com **dois QRs** e o IP certo já embutido:

- **QR amarelo (1 · primeira vez)** → abre o setup HTTP, que mostra o assistente de instalação do certificado
- **QR verde (2 · já instalei)** → abre direto o app seguro (HTTPS), **com o PIN de pareamento já embutido**

Na maioria dos PCs (sem Tailscale) aparece **um card com dois QRs**; com Tailscale ativo, dois cards.

Aponte a câmera do celular e siga. Se preferir digitar o IP na mão, os endereços estão logo abaixo de cada QR — nesse caso informe também o **PIN** mostrado no topo da página (em **⚙ Ajustes → PIN**). (Rodando escondido pelo Task Scheduler não há console, então nada abre sozinho — acesse `http://127.0.0.1:8080/qr` no PC quando quiser. Ou simplesmente **rode o `pc-remote.exe` de novo**: detectando que já há uma instância ativa, ele apenas abre essa página de conexão no navegador em vez de tentar subir de novo.)

---

## Instalação no celular (Android) — passo a passo

1. **No PC**, abra o **Controlinho** (Store) ou rode o `pc-remote.exe` (standalone). Com console, ele mostra os endereços e abre a página de QR:
   ```
   phone setup:  http://SEU_IP:8080   →  install the CA, then open the HTTPS link
   phone app:    https://SEU_IP:8443
   connect page (QR): http://127.0.0.1:8080/qr
   ```

2. **No celular**, escaneie o **QR amarelo** (ou abra `http://SEU_IP:8080`). Como essa origem HTTP não é um contexto seguro (não dá para controlar o PC nem instalar como app por ela), aparece um **assistente de configuração inicial** em tela cheia — só na primeira vez neste aparelho — com 3 passos.

3. **Passo 1 — Baixar o certificado:** toque em **"Baixar certificado"** (baixa `pc-remote-ca.crt`).

4. **Passo 2 — Instalar no aparelho:** instale a CA em **Ajustes → Segurança → Mais ajustes → Instalar certificado → Certificado CA** (o caminho varia por fabricante; procure por "Instalar certificado" / "Credenciais"). O Android deve avisar que é um **certificado CA** — se ele falar em "certificado de usuário", algo deu errado. De volta ao assistente, toque em **"Já instalei — verificar"**: ele testa a confiança na hora (tenta carregar um recurso HTTPS do PC) e, se a CA estiver confiável, libera o passo 3. Se ainda não detectar, confira a instalação e tente de novo.

5. **Passo 3 — Abrir o app seguro:** toque em **"Abrir app seguro"** (vai para `https://SEU_IP:8443`, com o **PIN de pareamento já embutido**). Agora carrega com cadeado, sem aviso. *(Se você já tinha configurado antes, o assistente detecta a confiança sozinho ao abrir e pula direto para este passo; o atalho **"Já configurei — abrir versão segura"** no rodapé também leva direto.)*

6. No menu do Chrome → **"Instalar app" / "Adicionar à tela inicial"**. Vira um ícone em tela cheia, sem barra do navegador.

> **iPhone/Safari:** "Adicionar à Tela de Início" funciona mesmo sem o certificado (vira atalho em tela cheia). Para o ícone abrir a versão segura, instale a CA em **Ajustes → Geral → Gerenciamento VPN e Dispositivo** e ative a confiança em **Ajustes → Geral → Sobre → Confiança de Certificado**.

---

## Uso

- **Configurar o IP:** toque em **⚙ Ajustes** e informe `host:porta` (ex.: `SEU_IP:8443`) e o **PIN** (mostrado na página `/qr`; ao escanear o QR ele já vem preenchido). Fica salvo no `localStorage`. No mesmo painel dá pra ajustar **sensibilidade do cursor**, **velocidade de rolagem**, **rolagem natural** (inverte a direção do scroll), **"Pressionar Enter ao enviar texto"** e ligar/desligar a **vibração** (haptics ao tocar).
- **Trackpad:** 1 dedo move · toque = clique esquerdo · toque com 2 dedos = clique direito · 2 dedos arrastando = rolar. O movimento tem **aceleração**: gestos lentos são precisos e flicks rápidos percorrem mais tela. O botão **✊ Arrastar** segura o botão esquerdo: ligue, toque e mova para arrastar janelas/seleções; solte o dedo para soltar.

### Argumentos

| Flag         | Padrão           | Descrição                                            |
|--------------|------------------|------------------------------------------------------|
| `-http`      | `0.0.0.0:8080`   | Porta HTTP (setup + download da CA)                  |
| `-https`     | `0.0.0.0:8443`   | Porta HTTPS (app + `wss://`)                         |
| `-install`   | —                | Auto-início (logon) + firewall, e inicia             |
| `-uninstall` | —                | Remove o auto-início e o firewall                    |
| `-version`   | —                | Mostra a versão e sai                                |

---

## Inicialização automática com o Windows

A lógica de instalação vive **no próprio binário** — não há instalador separado:

```bat
pc-remote.exe -install     :: registra no logon + abre o firewall, e já inicia
pc-remote.exe -uninstall   :: remove a tarefa e a regra de firewall
```

(O `install.bat` é só um atalho de duplo-clique que chama `-install`.)

Registra uma tarefa no **Task Scheduler** (`pc-remote`, trigger `OnLogon`, usuário atual, sem admin) e já inicia o servidor. A regra de firewall (`8080`/`8443`) precisa de admin: ele tenta direto e, se não tiver permissão, **se auto-eleva via UAC** só para criar a regra. É reexecutável.

> **Por que uma tarefa de logon e não um serviço do Windows?** Injeção de teclado/mouse só funciona a partir da **sessão interativa** do usuário. Um serviço roda na *Session 0*, isolada, e **não conseguiria** controlar o desktop — por isso a tarefa roda como o usuário logado.

> **Na versão da Microsoft Store (MSIX)** não existem as flags `-install`/`-uninstall`: a Store cuida de instalar/desinstalar, e o auto-início é o **mesmo toggle da bandeja** ("Iniciar com o Windows"), que usa o `StartupTask` do Windows em vez do Task Scheduler.

### 🔔 Ícone na bandeja

Rodando, o app coloca um **ícone na bandeja do sistema** (system tray) com:

- **Abrir página de conexão** — mostra os QR codes para parear o celular
- **Iniciar com o Windows** — liga/desliga o auto-início (equivale a `-install`/`-uninstall`)
- **Sair** — encerra o servidor

Isso resolve o "será que está rodando?" do modo silencioso: mesmo na build sem console (`-H windowsgui`), a bandeja é o caminho para abrir os QRs e encerrar.

Há **proteção contra instância dupla:** se você rodar o `pc-remote.exe` enquanto outra instância já está no ar, ele não tenta subir de novo — apenas abre a página de conexão (`http://127.0.0.1:8080/qr`) no navegador. E se alguma porta (`8080`/`8443`) estiver ocupada por outro programa, o servidor mostra uma mensagem clara em vez de uma falha críptica.

---

## Segurança

- **Só aceita conexões da rede local / Tailscale:** loopback, `10/8`, `172.16/12`, `192.168/16`, Tailscale `100.64/10`, IPv6 ULA. Qualquer outra origem recebe `403`.
- **Validação de origem (anti DNS-rebinding / CSRF):** o WebSocket só aceita upgrades cujo `Origin` bate com o host requisitado. Assim, um site malicioso aberto no celular (mesmo estando na sua LAN) **não** consegue abrir socket e controlar o PC — o `Origin` dele seria outro domínio.
- **HTTPS confiável** via CA local (acima). A chave privada não sai do PC e fica num diretório por usuário e local da máquina (`%LocalAppData%\pc-remote`, fora do controle de versão).
- **PIN de pareamento:** além da topologia de rede, todo WebSocket exige um **token** secreto (gerado uma vez e guardado em `%LocalAppData%\pc-remote`, junto da CA). Escaneando o QR verde o PIN já vai junto (zero atrito); para digitar o IP na mão, informe o **PIN** mostrado na página `/qr`. Estar na rede passa a ser necessário, mas não suficiente.
- **Topologia de rede ainda vale:** **não exponha as portas para a internet.** Para acesso externo, use Tailscale (o range `100.64/10` já é aceito).

> ⚠️ Ações de **energia** (desligar/reiniciar) não têm desfazer — confirme com cuidado.

---

## Protocolo WebSocket

Cada frame é um JSON (objeto ou array de objetos para batch). Erros vêm como `{"type":"...","ok":false,"error":"..."}`.

| type             | campos                          | efeito                                  |
|------------------|---------------------------------|-----------------------------------------|
| `mouse_move`     | `dx`, `dy`                      | move o cursor (relativo, px)            |
| `mouse_move_abs` | `dx`, `dy`                      | move o cursor (absoluto, px)            |
| `mouse_click`    | `button`: left/right/middle     | clique                                  |
| `mouse_down`     | `button`                        | segura o botão (para arrastar)          |
| `mouse_up`       | `button`                        | solta o botão                           |
| `mouse_scroll`   | `delta`                         | scroll (+ = cima, − = baixo)            |
| `key`            | `key`                           | tecla (ex.: `space`, `up`, `volumeup`)  |
| `shortcut`       | `keys[]`                        | combinação (ex.: `["ctrl","w"]`)        |
| `type`           | `text`                          | digita string (Unicode)                 |
| `volume`         | `action`: up/down/mute          | volume                                  |
| `media`          | `action`: play_pause/next/prev  | mídia                                   |
| `power`          | `action`: monitor_off/sleep/restart/shutdown | energia                    |
| `ping`           | —                               | keepalive                               |

Teclas reconhecidas: `ctrl alt shift win`, `enter esc tab backspace space del insert home end pageup pagedown` + setas (`up down left right`), `f1`–`f12`, `volumeup/down/mute`, `mediaplay/next/prev/stop`, `browserback/forward`, e qualquer caractere ASCII imprimível. As teclas estendidas (setas, navegação, mídia) são enviadas com `KEYEVENTF_EXTENDEDKEY`, então funcionam corretamente independente do Num Lock.

---

## ⚙️ Decisão de implementação (input sem CGO)

O prompt original pedia `github.com/go-vgo/robotgo` e `github.com/itchyny/volume-go`. **Ambos exigem CGO + MinGW (GCC)**, o que quebraria o "binário único, zero instalação". Em vez disso, o input chama a **Win32 API diretamente via `syscall`** (`mouse_event`, `keybd_event`, `MapVirtualKeyW`, `SendMessage`) — mesmo subsistema, sem GCC, binário menor. Volume/mídia via teclas de mídia do Windows (funcionam com qualquer saída de áudio).

Dependências (todas Go puro, **sem CGO** na build Windows): `github.com/gorilla/websocket` (WebSocket), `github.com/skip2/go-qrcode` (QR) e `fyne.io/systray` (ícone na bandeja — no Windows usa `golang.org/x/sys`, sem CGO). O systray só é importado em arquivos com build tag `windows`, então a build de desenvolvimento em Linux/macOS (`CGO_ENABLED=0`) continua compilando.

---

## Estrutura

```
pc-remote/
├── main.go              # Servidor HTTP+HTTPS, WebSocket, dispatch, allowlist, CheckOrigin
├── token.go             # PIN/token de pareamento (gera/persiste/valida) — exigido no WebSocket
├── tlsx.go              # CA local em %LocalAppData% (+ migração) + emissão dinâmica do certificado (mkcert-style), Go puro
├── qr.go                # Página /qr + QR no terminal + auto-open do navegador
├── input_windows.go     # Input via user32.dll (mouse/teclado/texto/monitor)
├── input_stub.go        # Stub no-op para build em Linux/macOS (dev)
├── install_windows.go   # [build .exe] Auto-início: Task Scheduler + firewall (auto-elevação UAC)
├── install_store.go     # [build store] Auto-início via WinRT StartupTask (MSIX)
├── install_other.go     # Stubs de install para Linux/macOS (dev)
├── installcli_desktop.go# [build .exe] Flags -install/-uninstall/-setupfw
├── installcli_store.go  # [build store] Sem flags de install (a Store instala)
├── tray_windows.go      # Ícone na bandeja (fyne.io/systray)
├── tray_other.go        # Sem bandeja fora do Windows; bloqueia no contexto (dev)
├── gen_icons.go         # Gerador de ícones do PWA/bandeja + assets MSIX (go run gen_icons.go)
├── go.mod / go.sum
├── client/              # PWA single-file + manifest + service worker + ícones (PNG + icon.ico)
├── packaging/           # MSIX (Microsoft Store): manifesto, assets, scripts de build/teste, listagem
├── PRIVACY.md           # Política de privacidade (publicada na Store)
├── install.bat          # Atalho de duplo-clique para `pc-remote.exe -install` (build .exe)
└── README.md
```

---

## Solução de problemas

- **Android não oferece "Instalar app":** confirme que (a) instalou a CA como **certificado CA** (não de usuário), (b) está na URL **HTTPS** (`:8443`), e (c) o cadeado aparece sem aviso. Sem isso o Chrome não registra o service worker.
- **"Sua conexão não é particular" no HTTPS:** a CA ainda não foi instalada/confiada no celular. Volte ao passo de instalação do certificado.
- **Não conecta:** PC e celular na mesma Wi-Fi; firewall liberando `8080`/`8443`; IP correto em **⚙ Ajustes**. Se aparecer **"informe o PIN nos ajustes"**, o token de pareamento está faltando — escaneie o QR verde de novo ou digite o **PIN** (mostrado em `/qr`) em **⚙ Ajustes**.
- **Mouse/teclado não responde:** rode como usuário logado (input só funciona em sessão interativa).
- **Trocou de rede e o HTTPS quebrou:** normalmente **não** precisa mais reiniciar — o servidor reemite o certificado HTTPS sob demanda quando o IP do PC muda (novo Wi-Fi / DHCP) ou quando o certificado se aproxima do vencimento. A CA continua válida (não precisa reinstalar no celular). Se mesmo assim não pegar, reinicie o `pc-remote.exe`.

---

## Licença

Uso pessoal. Win32 API é padrão do Windows; `gorilla/websocket` é BSD-2.
