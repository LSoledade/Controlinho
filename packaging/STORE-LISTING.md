# Listagem da Microsoft Store — Controlinho

> Copie/cole no Partner Center (Listagem da Store, idioma **pt-BR**).
> Política de privacidade: ver [`../PRIVACY.md`](../PRIVACY.md) (hospede numa URL pública).

## Nome do produto
Controlinho

## Descrição curta
Use o celular como trackpad, teclado e controle de mídia do seu PC, pela rede local. Sem nuvem.

## Descrição

Controlinho transforma seu celular em um controle completo para o seu PC com Windows —
pela sua rede Wi-Fi ou Tailscale, sem nuvem e sem complicação.

Abra o app no PC, escaneie o QR Code com o celular e pronto: seu telefone vira trackpad,
teclado e controle remoto.

Recursos:
• Trackpad com gestos — mover, clicar, clique direito (2 dedos), rolar e arrastar, com aceleração ajustável.
• Teclado e atalhos — digite no PC e dispare atalhos (copiar, colar, Alt+Tab e mais).
• Mídia e volume — play/pause, faixa anterior/próxima, volume e mudo.
• Energia — desligar, reiniciar, suspender ou apagar o monitor (com confirmação).
• Instalável como app (PWA) no celular, em tela cheia.

Privacidade e segurança em primeiro lugar:
• 100% local — a comunicação é direta entre o celular e o seu PC. Sem nuvem, sem contas, sem anúncios, sem rastreamento.
• Conexão protegida por PIN de pareamento.
• Só aceita conexões da sua rede local ou Tailscale.

Importante (passo único de certificado):
Para uma conexão segura (HTTPS) na rede local, na primeira vez o celular instala um
certificado gerado pelo seu próprio PC. É um passo rápido, feito uma única vez, e a chave
nunca sai do seu computador. O app guia você no processo.

Requisitos:
• PC com Windows 10/11.
• Celular (Android/iOS) com navegador, na mesma rede do PC (ou via Tailscale).

## Novidades nesta versão (What's new)
Primeira versão na Microsoft Store. Controle seu PC pelo celular: trackpad, teclado,
mídia e energia — tudo local e protegido por PIN.

## Termos de pesquisa (search terms)
controle remoto, trackpad, mouse pelo celular, teclado, controle de mídia, controlar PC,
remote, tailscale, rede local, PWA

## Classificação etária
Sem conteúdo sensível — deve receber classificação livre no questionário.

## Notas para a certificação (Notes for certification)
- **runFullTrust:** controle remoto do desktop a partir do celular na rede local; requer
  injeção de input via Win32 (user32.dll), incompatível com sandbox.
- **Autenticação:** PIN de pareamento exigido em toda conexão WebSocket; o servidor só
  aceita conexões de rede local/Tailscale (loopback, 10/172.16/192.168, 100.64/10, ULA IPv6).
- **CA local:** o app instala uma autoridade certificadora apenas no dispositivo do
  próprio usuário, por escolha dele, para HTTPS confiável na LAN; a chave privada não sai
  do PC. Divulgado na descrição e na política de privacidade.
- **Sem coleta de dados** e sem servidores do desenvolvedor.
