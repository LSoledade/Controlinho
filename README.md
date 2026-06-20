# 🖥️ PC Remote

Controle o PC Windows pelo celular, na rede local. Servidor Go + cliente PWA num único binário, sem nuvem, sem dependências externas, sem instalação extra.

- **Trackpad** — mover, clique esquerdo/direito/meio, **arrastar**, scroll com 2 dedos, sensibilidade ajustável
- **Mídia & Volume** — play/pause, próxima, anterior, volume +/−, mudo, voltar/avançar navegador
- **Teclado / Atalhos** — digitar texto (Unicode), Ctrl+C/V/Z…, Alt+F4, Win+L, **setas e navegação (D-pad)**
- **Energia** — desligar monitor, suspender, reiniciar, desligar (com confirmação)

Tudo num executável único que sobe o servidor **e** serve a interface, com **HTTPS confiável** para instalar como app no celular.

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

Para o Chrome no Android oferecer **"Instalar app"** e registrar o service worker, a página precisa estar num **contexto seguro** (HTTPS confiável). A exceção `localhost` **não** vale para um IP de LAN tipo `192.168.0.70` — sobre `http://` o Android não instala como app de verdade.

A solução, sem depender de nuvem ou serviço de terceiros, é a mesma técnica do [`mkcert`](https://github.com/FiloSottile/mkcert):

1. Na **primeira execução**, o servidor gera uma **autoridade certificadora (CA) local** própria (`pc-remote-ca.crt` / `.key`, guardados ao lado do `.exe`).
2. A cada boot ele emite um certificado HTTPS para os IPs atuais do PC (LAN + Tailscale), assinado por essa CA.
3. Você instala a CA no celular **uma única vez**. A partir daí o Chrome confia em tudo que o servidor assina → cadeado verde, contexto seguro, service worker e instalação do PWA funcionando.

A chave privada da CA **nunca sai do PC**.

> ⚠️ **Não compartilhe o `pc-remote-ca.key`.** Quem tiver esse arquivo pode emitir certificados confiáveis para o seu celular. Ele já fica fora do controle de versão (veja `.gitignore`).

---

## Build

Requisitos: **Go 1.21+** (testado com Go 1.25). Windows/amd64. Não precisa de GCC.

```bat
:: Versão de release (binário enxuto)
go build -ldflags "-s -w" -o pc-remote.exe .

:: Sem janela de console (inicia silencioso pelo Task Scheduler)
go build -ldflags "-s -w -H windowsgui" -o pc-remote.exe .
```

O cliente é embutido no binário via `//go:embed`, então qualquer mudança em `client/` exige rebuild.

---

## Conectar pelo QR Code (jeito mais fácil)

Quando você roda o `pc-remote.exe` **com console** (run manual), ele:

- abre automaticamente a **página de conexão** no navegador do PC (`http://127.0.0.1:8080/qr`), e
- desenha um **QR Code no próprio terminal**.

A página mostra **dois QRs por rede** (Wi-Fi local e Tailscale), já com o IP certo embutido:

- **QR amarelo (1 · primeira vez)** → abre o setup HTTP para instalar o certificado
- **QR verde (2 · já instalei)** → abre direto o app seguro (HTTPS)

Aponte a câmera do celular e siga. Se preferir digitar, os endereços estão logo abaixo de cada QR. (Rodando escondido pelo Task Scheduler não há console, então nada abre sozinho — acesse `http://127.0.0.1:8080/qr` no PC quando quiser.)

---

## Instalação no celular (Android) — passo a passo

1. **No PC**, rode `pc-remote.exe`. O console mostra os endereços e abre a página de QR:
   ```
   phone setup:  http://192.168.0.70:8080   →  install the CA, then open the HTTPS link
   phone app:    https://192.168.0.70:8443
   connect page (QR): http://127.0.0.1:8080/qr
   ```

2. **No celular**, escaneie o **QR amarelo** (ou abra `http://192.168.0.70:8080`). O app já funciona para controle, e aparece um banner **"📲 Instalar como app"**.

3. Toque em **"1 · Baixar certificado"** (baixa `pc-remote-ca.crt`).

4. Instale a CA: **Ajustes → Segurança → Mais ajustes → Instalar certificado → Certificado CA** (o caminho varia por fabricante; procure por "Instalar certificado" / "Credenciais"). O Android deve avisar que é um **certificado CA** — se ele falar em "certificado de usuário", algo deu errado.

5. Volte ao app e toque em **"2 · Abrir versão segura"** (vai para `https://192.168.0.70:8443`). Agora carrega com cadeado, sem aviso.

6. No menu do Chrome → **"Instalar app" / "Adicionar à tela inicial"**. Vira um ícone em tela cheia, sem barra do navegador.

> **iPhone/Safari:** "Adicionar à Tela de Início" funciona mesmo sem o certificado (vira atalho em tela cheia). Para o ícone abrir a versão segura, instale a CA em **Ajustes → Geral → Gerenciamento VPN e Dispositivo** e ative a confiança em **Ajustes → Geral → Sobre → Confiança de Certificado**.

---

## Uso

- **Configurar o IP:** toque em **⚙ Ajustes** e informe `host:porta` (ex.: `192.168.0.70:8443`). Fica salvo no `localStorage`. Também dá pra ajustar a **sensibilidade do cursor** aí.
- **Trackpad:** 1 dedo move · toque = clique esquerdo · toque com 2 dedos = clique direito · 2 dedos arrastando = rolar. O botão **✊ Arrastar** segura o botão esquerdo: ligue, toque e mova para arrastar janelas/seleções; solte o dedo para soltar.

### Argumentos

| Flag        | Padrão           | Descrição                          |
|-------------|------------------|------------------------------------|
| `-http`     | `0.0.0.0:8080`   | Porta HTTP (setup + download da CA)|
| `-https`    | `0.0.0.0:8443`   | Porta HTTPS (app + `wss://`)       |
| `-version`  | —                | Mostra a versão e sai              |

---

## Inicialização automática com o Windows

```bat
install.bat
```

Registra uma tarefa no **Task Scheduler** (`pc-remote`, trigger `OnLogon`, usuário atual, sem admin), tenta abrir o firewall para as portas `8080`/`8443` (se rodar como admin) e já inicia o servidor. É reexecutável.

---

## Segurança

- **Só aceita conexões da rede local / Tailscale:** loopback, `10/8`, `172.16/12`, `192.168/16`, Tailscale `100.64/10`, IPv6 ULA. Qualquer outra origem recebe `403`.
- **Validação de origem (anti DNS-rebinding / CSRF):** o WebSocket só aceita upgrades cujo `Origin` bate com o host requisitado. Assim, um site malicioso aberto no celular (mesmo estando na sua LAN) **não** consegue abrir socket e controlar o PC — o `Origin` dele seria outro domínio.
- **HTTPS confiável** via CA local (acima). A chave privada não sai do PC.
- **Sem senha:** o modelo é confiar na topologia de rede. **Não exponha as portas para a internet.** Para acesso externo, use Tailscale (o range `100.64/10` já é aceito).

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

O prompt original pedia `github.com/go-vgo/robotgo` e `github.com/itchyny/volume-go`. **Ambos exigem CGO + MinGW (GCC)**, o que quebraria o "binário único, zero instalação". Em vez disso, o input chama a **Win32 API diretamente via `syscall`** (`mouse_event`, `keybd_event`, `MapVirtualKeyW`, `SendMessage`) — mesmo subsistema, sem GCC, binário menor. Volume/mídia via teclas de mídia do Windows (funcionam com qualquer saída de áudio). Única dependência externa: `github.com/gorilla/websocket` (pura Go).

---

## Estrutura

```
pc-remote/
├── main.go            # Servidor HTTP+HTTPS, WebSocket, dispatch, allowlist, CheckOrigin
├── tlsx.go            # CA local + emissão de certificado (mkcert-style), Go puro
├── qr.go              # Página /qr + QR no terminal + auto-open do navegador
├── input_windows.go   # Input via user32.dll (mouse/teclado/texto/monitor)
├── input_stub.go      # Stub no-op para build em Linux/macOS (dev)
├── gen_icons.go       # Gerador de ícones do PWA (go run gen_icons.go)
├── go.mod / go.sum
├── client/            # PWA single-file + manifest + service worker + ícones
├── install.bat        # Task Scheduler (logon) + firewall
└── README.md
```

---

## Solução de problemas

- **Android não oferece "Instalar app":** confirme que (a) instalou a CA como **certificado CA** (não de usuário), (b) está na URL **HTTPS** (`:8443`), e (c) o cadeado aparece sem aviso. Sem isso o Chrome não registra o service worker.
- **"Sua conexão não é particular" no HTTPS:** a CA ainda não foi instalada/confiada no celular. Volte ao passo de instalação do certificado.
- **Não conecta:** PC e celular na mesma Wi-Fi; firewall liberando `8080`/`8443`; IP correto em **⚙ Ajustes**.
- **Mouse/teclado não responde:** rode como usuário logado (input só funciona em sessão interativa).
- **Trocou de rede e o HTTPS quebrou:** o certificado é reemitido a cada boot com os IPs atuais; reinicie o `pc-remote.exe`. A CA continua válida (não precisa reinstalar no celular).

---

## Licença

Uso pessoal. Win32 API é padrão do Windows; `gorilla/websocket` é BSD-2.
